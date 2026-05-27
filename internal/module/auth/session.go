package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/accesstoken"
	"admin_back_go/internal/platform/database"
	"admin_back_go/internal/platform/redisclient"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Session token hashing.
var ErrUnsafePepper = errors.New("token pepper is empty or unsafe")

func HashToken(token string, pepper string) (string, error) {
	if pepper == "" || pepper == "change_me_to_long_random" {
		return "", ErrUnsafePepper
	}

	sum := sha256.Sum256([]byte(token + "|" + pepper))
	return hex.EncodeToString(sum[:]), nil
}

// Session persistence model and cache serialization.
type Session struct {
	ID               int64      `gorm:"column:id"`
	UserID           int64      `gorm:"column:user_id"`
	AccessTokenHash  string     `gorm:"column:access_token_hash"`
	RefreshTokenHash string     `gorm:"column:refresh_token_hash"`
	Platform         string     `gorm:"column:platform"`
	DeviceID         string     `gorm:"column:device_id"`
	IP               string     `gorm:"column:ip"`
	UserAgent        string     `gorm:"column:ua"`
	LastSeenAt       time.Time  `gorm:"column:last_seen_at"`
	ExpiresAt        time.Time  `gorm:"column:expires_at"`
	RefreshExpiresAt time.Time  `gorm:"column:refresh_expires_at"`
	RevokedAt        *time.Time `gorm:"column:revoked_at"`
	IsDel            int        `gorm:"column:is_del"`
}

func (Session) TableName() string {
	return "user_sessions"
}

func parseCachedSession(value string, loc *time.Location) (*Session, error) {
	parts := strings.Split(value, "|")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid cached session")
	}

	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	expiresAt, err := parseSessionTime(parts[1], loc)
	if err != nil {
		return nil, err
	}

	session := &Session{
		UserID:    userID,
		ExpiresAt: expiresAt,
		IP:        parts[2],
		Platform:  parts[3],
	}
	if len(parts) >= 5 {
		session.DeviceID = parts[4]
	}
	if len(parts) >= 6 && parts[5] != "" {
		session.ID, err = strconv.ParseInt(parts[5], 10, 64)
		if err != nil {
			return nil, err
		}
	}
	return session, nil
}

func cacheValue(session *Session) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s|%d",
		session.UserID,
		session.ExpiresAt.Format("2006-01-02 15:04:05"),
		session.IP,
		session.Platform,
		session.DeviceID,
		session.ID,
	)
}

func parseSessionTime(value string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, loc); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

// Session cache boundary.
type SessionCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

type SessionRedisCache struct {
	client *redis.Client
}

func NewSessionRedisCache(client *redisclient.Client) SessionCache {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &SessionRedisCache{client: client.Redis}
}

func (c *SessionRedisCache) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrSessionCacheNotConfigured
	}
	value, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return value, err
}

func (c *SessionRedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return ErrSessionCacheNotConfigured
	}
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *SessionRedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return ErrSessionCacheNotConfigured
	}
	return c.client.Expire(ctx, key, ttl).Err()
}

func (c *SessionRedisCache) Del(ctx context.Context, key string) error {
	if c == nil || c.client == nil {
		return ErrSessionCacheNotConfigured
	}
	return c.client.Del(ctx, key).Err()
}

// Session cache revocation helpers.
type SessionRevocationConfig struct {
	RedisPrefix string
}

type SessionRevocationService struct {
	cache SessionCache
	cfg   SessionRevocationConfig
}

func NewSessionRevocationService(cache SessionCache, cfg SessionRevocationConfig) *SessionRevocationService {
	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = "token:"
	}
	return &SessionRevocationService{cache: cache, cfg: cfg}
}

func (s *SessionRevocationService) RevokeCache(ctx context.Context, row Session) error {
	if s == nil || s.cache == nil {
		return ErrSessionCacheNotConfigured
	}

	if row.ID > 0 {
		if err := s.cache.Del(ctx, s.sessionCacheKey(row.ID)); err != nil {
			return err
		}
	}

	if row.ID <= 0 || row.UserID <= 0 || strings.TrimSpace(row.Platform) == "" {
		return nil
	}

	pointerKey := s.singleSessionPointerKey(row.Platform, row.UserID)
	current, err := s.cache.Get(ctx, pointerKey)
	if err != nil {
		return err
	}
	if sameSessionID(current, row.ID) {
		if err := s.cache.Del(ctx, pointerKey); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionRevocationService) RevokeCaches(ctx context.Context, rows []Session) error {
	for _, row := range rows {
		if err := s.RevokeCache(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionRevocationService) sessionCacheKey(sessionID int64) string {
	return s.cfg.RedisPrefix + "session:" + strconv.FormatInt(sessionID, 10)
}

func (s *SessionRevocationService) singleSessionPointerKey(platform string, userID int64) string {
	return s.cfg.RedisPrefix + "cur_sess:" + strings.ToLower(strings.TrimSpace(platform)) + ":" + strconv.FormatInt(userID, 10)
}

// Session repository.
type SessionRepository interface {
	FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error)
	FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error)
	FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error)
	Create(ctx context.Context, input SessionCreate) (int64, error)
	ListActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error)
	RevokeByUserPlatform(ctx context.Context, userID int64, platform string, revokedAt time.Time) error
	UpdateAccessToken(ctx context.Context, sessionID int64, accessHash string, expiresAt time.Time) error
	Rotate(ctx context.Context, sessionID int64, rotation SessionRotation) error
	Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error
}

type SessionCreate struct {
	UserID           int64
	AccessTokenHash  string
	RefreshTokenHash string
	Platform         string
	DeviceID         string
	IP               string
	UserAgent        string
	LastSeenAt       time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
}

type SessionRotation struct {
	AccessTokenHash  string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	LastSeenAt       time.Time
	IP               string
	UserAgent        string
}

func (r *SessionGormRepository) Create(ctx context.Context, input SessionCreate) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrSessionRepositoryNotConfigured
	}

	row := Session{
		UserID:           input.UserID,
		AccessTokenHash:  input.AccessTokenHash,
		RefreshTokenHash: input.RefreshTokenHash,
		Platform:         input.Platform,
		DeviceID:         input.DeviceID,
		IP:               input.IP,
		UserAgent:        input.UserAgent,
		LastSeenAt:       input.LastSeenAt,
		ExpiresAt:        input.ExpiresAt,
		RefreshExpiresAt: input.RefreshExpiresAt,
		IsDel:            commonNo,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *SessionGormRepository) ListActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}
	var sessions []Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		Order("id ASC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *SessionGormRepository) RevokeByUserPlatform(ctx context.Context, userID int64, platform string, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Update("revoked_at", revokedAt).Error
}

type SessionGormRepository struct {
	db *gorm.DB
}

func NewSessionGormRepository(client *database.Client) SessionRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &SessionGormRepository{db: client.Gorm}
}

func (r *SessionGormRepository) FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("id = ?", sessionID).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("expires_at > ?", now).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionGormRepository) FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("refresh_token_hash = ?", refreshHash).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionGormRepository) FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		Order("id DESC").
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionGormRepository) Rotate(ctx context.Context, sessionID int64, rotation SessionRotation) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"access_token_hash":  rotation.AccessTokenHash,
			"refresh_token_hash": rotation.RefreshTokenHash,
			"expires_at":         rotation.ExpiresAt,
			"refresh_expires_at": rotation.RefreshExpiresAt,
			"last_seen_at":       rotation.LastSeenAt,
			"ip":                 rotation.IP,
			"ua":                 rotation.UserAgent,
		}).Error
}

func (r *SessionGormRepository) UpdateAccessToken(ctx context.Context, sessionID int64, accessHash string, expiresAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"access_token_hash": accessHash,
			"expires_at":        expiresAt,
		}).Error
}

func (r *SessionGormRepository) Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Update("revoked_at", revokedAt).Error
}

// Session authenticator and token lifecycle.
var (
	ErrSessionCacheNotConfigured      = errors.New("session cache is not configured")
	ErrSessionRepositoryNotConfigured = errors.New("session repository is not configured")
)

type TokenInput struct {
	AccessToken string
	Platform    string
	DeviceID    string
	ClientIP    string
}

type Identity struct {
	UserID    int64
	SessionID int64
	Platform  string
}

type AuthPolicy struct {
	BindPlatform             bool
	BindDevice               bool
	BindIP                   bool
	SingleSessionPerPlatform bool
	MaxSessions              int
	AllowRegister            bool
	AccessTTL                time.Duration
	RefreshTTL               time.Duration
}

type PolicyProvider interface {
	Policy(ctx context.Context, platform string) (*AuthPolicy, error)
}

type AuthenticatorDeps struct {
	Config         config.TokenConfig
	Cache          SessionCache
	Repository     SessionRepository
	PolicyProvider PolicyProvider
	AccessCodec    accesstoken.Codec
	TokenPepper    string
	TokenGenerator TokenGenerator
	Now            func() time.Time
}

type Authenticator struct {
	cfg            config.TokenConfig
	cache          SessionCache
	repository     SessionRepository
	policyProvider PolicyProvider
	accessCodec    accesstoken.Codec
	tokenPepper    string
	tokenGenerator TokenGenerator
	now            func() time.Time
	loc            *time.Location
}

type TokenGenerator func(bytes int) (string, error)

type RefreshInput struct {
	RefreshToken string
	ClientIP     string
	UserAgent    string
}

type CreateInput struct {
	UserID    int64
	Platform  string
	DeviceID  string
	ClientIP  string
	UserAgent string
}

type TokenResult struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

func (a *Authenticator) Create(ctx context.Context, input CreateInput) (*TokenResult, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	if input.UserID <= 0 {
		return nil, apperror.BadRequest("无效的用户ID")
	}
	input.Platform = strings.TrimSpace(input.Platform)
	if input.Platform == "" {
		return nil, apperror.BadRequest("无效的平台标识")
	}
	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	policy, policyErr := a.policyForSession(ctx, input.Platform)
	if policyErr != nil {
		return nil, policyErr
	}

	now := a.now()
	refreshToken, refreshHash, refreshExpiresAt, tokenErr := a.issueRefreshToken(policy, now)
	if tokenErr != nil {
		return nil, tokenErr
	}

	if evictErr := a.evictSessions(ctx, input.UserID, input.Platform, policy, now); evictErr != nil {
		return nil, evictErr
	}

	sessionID, err := a.repository.Create(ctx, SessionCreate{
		UserID:           input.UserID,
		AccessTokenHash:  temporaryAccessTokenHash(sessionIDPlaceholder(input.UserID, input.Platform, now)),
		RefreshTokenHash: refreshHash,
		Platform:         input.Platform,
		DeviceID:         input.DeviceID,
		IP:               input.ClientIP,
		UserAgent:        input.UserAgent,
		LastSeenAt:       now,
		ExpiresAt:        now.Add(policy.AccessTTL),
		RefreshExpiresAt: refreshExpiresAt,
	})
	if err != nil {
		return nil, apperror.Internal("创建登录会话失败")
	}

	accessToken, accessHash, accessExpiresAt, accessErr := a.issueAccessToken(sessionID, input.UserID, input.Platform, input.DeviceID, policy, now)
	if accessErr != nil {
		return nil, accessErr
	}
	if err := a.repository.UpdateAccessToken(ctx, sessionID, accessHash, accessExpiresAt); err != nil {
		return nil, apperror.Internal("更新登录会话失败")
	}

	a.updateSingleSessionPointer(ctx, &Session{ID: sessionID, UserID: input.UserID, Platform: input.Platform})
	return &TokenResult{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int(policy.AccessTTL.Seconds()),
		RefreshExpiresIn: int(policy.RefreshTTL.Seconds()),
	}, nil
}

func NewAuthenticator(deps AuthenticatorDeps) *Authenticator {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	deps.Config = config.NormalizeTokenConfig(deps.Config)
	tokenGenerator := deps.TokenGenerator
	if tokenGenerator == nil {
		tokenGenerator = makeToken
	}
	return &Authenticator{
		cfg:            deps.Config,
		cache:          deps.Cache,
		repository:     deps.Repository,
		policyProvider: deps.PolicyProvider,
		accessCodec:    deps.AccessCodec,
		tokenPepper:    deps.TokenPepper,
		tokenGenerator: tokenGenerator,
		now:            now,
		loc:            time.Local,
	}
}

func (a *Authenticator) Authenticate(ctx context.Context, input TokenInput) (*Identity, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	if a.accessCodec == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	now := a.now()
	claims, err := a.accessCodec.Parse(input.AccessToken, now)
	if err != nil {
		return nil, apperror.Unauthorized("Token格式错误")
	}
	cacheKey := a.sessionCacheKey(claims.SessionID)
	if a.cache != nil {
		if session, cacheErr := a.sessionFromCache(ctx, cacheKey); cacheErr != nil {
			return nil, cacheErr
		} else if session != nil {
			if err := matchClaims(session, claims); err != nil {
				a.deleteCache(ctx, cacheKey)
				return nil, err
			}
			if policyErr := a.enforcePolicy(ctx, cacheKey, session, input); policyErr != nil {
				return nil, policyErr
			}
			return identityFromSession(session), nil
		}
	}

	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	session, repoErr := a.repository.FindValidByID(ctx, claims.SessionID, now)
	if repoErr != nil {
		return nil, apperror.Internal("Token会话查询失败")
	}
	if session == nil {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if !session.ExpiresAt.After(now) {
		return nil, apperror.Unauthorized("Token已过期")
	}
	if err := matchClaims(session, claims); err != nil {
		return nil, err
	}

	if policyErr := a.enforcePolicy(ctx, cacheKey, session, input); policyErr != nil {
		return nil, policyErr
	}

	if a.cache != nil {
		_ = a.cache.Set(ctx, cacheKey, cacheValue(session), a.cfg.SessionCacheTTL)
	}
	return identityFromSession(session), nil
}

func (a *Authenticator) Refresh(ctx context.Context, input RefreshInput) (*TokenResult, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	if strings.TrimSpace(input.RefreshToken) == "" {
		return nil, apperror.Unauthorized("缺少刷新令牌")
	}

	refreshHash, err := HashToken(input.RefreshToken, a.tokenPepper)
	if err != nil {
		return nil, apperror.Unauthorized("令牌格式错误")
	}
	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	now := a.now()
	session, repoErr := a.repository.FindValidByRefreshHash(ctx, refreshHash, now)
	if repoErr != nil {
		return nil, apperror.Internal("刷新令牌查询失败")
	}
	if session == nil {
		return nil, apperror.Unauthorized("刷新令牌无效或已过期")
	}
	if !session.RefreshExpiresAt.After(now) {
		return nil, apperror.Unauthorized("刷新令牌已过期，请重新登录")
	}

	policy, policyErr := a.policyForSession(ctx, session.Platform)
	if policyErr != nil {
		return nil, policyErr
	}
	if policy.SingleSessionPerPlatform {
		if singleErr := a.enforceSingleSessionForRefresh(ctx, session); singleErr != nil {
			return nil, singleErr
		}
	}

	accessToken, accessHash, accessExpiresAt, accessErr := a.issueAccessToken(session.ID, session.UserID, session.Platform, session.DeviceID, policy, now)
	if accessErr != nil {
		return nil, accessErr
	}
	refreshToken, refreshHash, _, tokenErr := a.issueRefreshToken(policy, now)
	if tokenErr != nil {
		return nil, tokenErr
	}

	rotation := SessionRotation{
		AccessTokenHash:  accessHash,
		RefreshTokenHash: refreshHash,
		ExpiresAt:        accessExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
		LastSeenAt:       now,
		IP:               input.ClientIP,
		UserAgent:        input.UserAgent,
	}
	if err := a.repository.Rotate(ctx, session.ID, rotation); err != nil {
		return nil, apperror.Internal("刷新令牌更新失败")
	}

	a.deleteCache(ctx, a.sessionCacheKey(session.ID))
	a.updateSingleSessionPointer(ctx, session)

	return &TokenResult{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int(policy.AccessTTL.Seconds()),
		RefreshExpiresIn: int(policy.RefreshTTL.Seconds()),
	}, nil
}

func (a *Authenticator) Logout(ctx context.Context, accessToken string) *apperror.Error {
	if a == nil || strings.TrimSpace(accessToken) == "" {
		return nil
	}

	if a.accessCodec == nil {
		return nil
	}
	claims, err := a.accessCodec.Parse(accessToken, a.now())
	if err != nil {
		return nil
	}
	if a.repository == nil {
		return nil
	}

	session, err := a.repository.FindValidByID(ctx, claims.SessionID, a.now())
	if err != nil || session == nil {
		return nil
	}
	if err := a.repository.Revoke(ctx, session.ID, a.now()); err != nil {
		return apperror.Internal("退出登录失败")
	}

	a.deleteCache(ctx, a.sessionCacheKey(session.ID))
	a.clearPointerIfMatches(ctx, session)
	return nil
}

func (a *Authenticator) evictSessions(ctx context.Context, userID int64, platform string, policy *AuthPolicy, now time.Time) *apperror.Error {
	if policy == nil {
		return nil
	}
	if policy.SingleSessionPerPlatform {
		oldSessions, err := a.repository.ListActiveByUserPlatform(ctx, userID, platform, now)
		if err != nil {
			return apperror.Internal("查询旧会话失败")
		}
		if err := a.repository.RevokeByUserPlatform(ctx, userID, platform, now); err != nil {
			return apperror.Internal("撤销旧会话失败")
		}
		a.deleteSessionCaches(ctx, oldSessions)
		return nil
	}
	if policy.MaxSessions <= 0 {
		return nil
	}

	activeSessions, err := a.repository.ListActiveByUserPlatform(ctx, userID, platform, now)
	if err != nil {
		return apperror.Internal("查询旧会话失败")
	}
	overCount := len(activeSessions) - policy.MaxSessions + 1
	if overCount <= 0 {
		return nil
	}
	for _, oldSession := range activeSessions[:overCount] {
		if err := a.repository.Revoke(ctx, oldSession.ID, now); err != nil {
			return apperror.Internal("撤销旧会话失败")
		}
		a.deleteCache(ctx, a.sessionCacheKey(oldSession.ID))
	}
	return nil
}

func (a *Authenticator) deleteSessionCaches(ctx context.Context, sessions []Session) {
	for _, session := range sessions {
		a.deleteCache(ctx, a.sessionCacheKey(session.ID))
	}
}

func (a *Authenticator) sessionFromCache(ctx context.Context, cacheKey string) (*Session, *apperror.Error) {
	value, err := a.cache.Get(ctx, cacheKey)
	if err != nil {
		return nil, apperror.Internal("Token缓存读取失败")
	}
	if value == "" {
		return nil, nil
	}

	session, err := parseCachedSession(value, a.loc)
	if err != nil {
		_ = a.cache.Del(ctx, cacheKey)
		return nil, nil
	}
	if !session.ExpiresAt.After(a.now()) {
		_ = a.cache.Del(ctx, cacheKey)
		return nil, apperror.Unauthorized("Token已过期")
	}
	_ = a.cache.Expire(ctx, cacheKey, a.cfg.SessionCacheTTL)
	return session, nil
}

func (a *Authenticator) enforcePolicy(ctx context.Context, cacheKey string, session *Session, input TokenInput) *apperror.Error {
	if session == nil {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if a.policyProvider == nil {
		return apperror.Unauthorized("平台策略未配置")
	}

	currentPlatform := strings.TrimSpace(input.Platform)
	if currentPlatform == "" {
		return apperror.BadRequest("无效的平台标识")
	}
	currentPolicy, err := a.policyProvider.Policy(ctx, currentPlatform)
	if err != nil {
		return apperror.Internal("平台策略查询失败")
	}
	if currentPolicy == nil {
		return apperror.BadRequest("无效的平台标识")
	}

	policy := currentPolicy
	if session.Platform != currentPlatform {
		policy, err = a.policyProvider.Policy(ctx, session.Platform)
		if err != nil {
			return apperror.Internal("平台策略查询失败")
		}
		if policy == nil {
			return apperror.Unauthorized("平台未配置或已禁用")
		}
	}

	if policy.BindPlatform && !strings.EqualFold(session.Platform, currentPlatform) {
		return apperror.Unauthorized("平台不匹配")
	}
	if policy.BindDevice && session.DeviceID != "" {
		if input.DeviceID == "" || input.DeviceID != session.DeviceID {
			return apperror.Unauthorized("设备变更，请重新登录")
		}
	}
	if policy.BindIP && session.IP != input.ClientIP {
		a.deleteCache(ctx, cacheKey)
		return apperror.Unauthorized("IP地址变动")
	}
	if policy.SingleSessionPerPlatform {
		if err := a.enforceSingleSession(ctx, cacheKey, session); err != nil {
			return err
		}
	}
	return nil
}

func (a *Authenticator) policyForSession(ctx context.Context, platform string) (*AuthPolicy, *apperror.Error) {
	if a.policyProvider == nil {
		return nil, apperror.Unauthorized("平台策略未配置")
	}
	policy, err := a.policyProvider.Policy(ctx, platform)
	if err != nil {
		return nil, apperror.Internal("平台策略查询失败")
	}
	if policy == nil {
		return nil, apperror.Unauthorized("平台未配置或已禁用")
	}
	if policy.AccessTTL <= 0 {
		return nil, apperror.Internal("认证平台Token有效期未配置")
	}
	if policy.RefreshTTL <= 0 {
		return nil, apperror.Internal("认证平台Token有效期未配置")
	}
	return policy, nil
}

func (a *Authenticator) enforceSingleSession(ctx context.Context, cacheKey string, session *Session) *apperror.Error {
	if a.cache == nil {
		return apperror.Unauthorized("Token认证未配置")
	}

	pointerKey := a.singleSessionPointerKey(session.Platform, session.UserID)
	allowedID, err := a.cache.Get(ctx, pointerKey)
	if err != nil {
		return apperror.Internal("单端登录指针读取失败")
	}

	if allowedID == "" {
		latest, latestErr := a.latestActiveSession(ctx, session.UserID, session.Platform)
		if latestErr != nil {
			return latestErr
		}
		if latest != nil {
			allowedID = strconv.FormatInt(latest.ID, 10)
			_ = a.cache.Set(ctx, pointerKey, allowedID, a.cfg.SingleSessionPointerTTL)
		}
	} else if !sameSessionID(allowedID, session.ID) {
		latest, latestErr := a.latestActiveSession(ctx, session.UserID, session.Platform)
		if latestErr != nil {
			return latestErr
		}
		if latest != nil && !sameSessionID(allowedID, latest.ID) {
			allowedID = strconv.FormatInt(latest.ID, 10)
			_ = a.cache.Set(ctx, pointerKey, allowedID, a.cfg.SingleSessionPointerTTL)
		} else if latest == nil {
			allowedID = ""
		}
	}

	if allowedID != "" && !sameSessionID(allowedID, session.ID) {
		a.deleteCache(ctx, cacheKey)
		return apperror.Unauthorized("账号已在其他设备登录")
	}
	return nil
}

func (a *Authenticator) enforceSingleSessionForRefresh(ctx context.Context, session *Session) *apperror.Error {
	if a.cache == nil {
		return apperror.Unauthorized("Token认证未配置")
	}
	pointerKey := a.singleSessionPointerKey(session.Platform, session.UserID)
	allowedID, err := a.cache.Get(ctx, pointerKey)
	if err != nil {
		return apperror.Internal("单端登录指针读取失败")
	}
	if allowedID == "" {
		latest, latestErr := a.latestActiveSession(ctx, session.UserID, session.Platform)
		if latestErr != nil {
			return latestErr
		}
		if latest != nil {
			allowedID = strconv.FormatInt(latest.ID, 10)
			_ = a.cache.Set(ctx, pointerKey, allowedID, a.cfg.SingleSessionPointerTTL)
		}
	}
	if allowedID != "" && !sameSessionID(allowedID, session.ID) {
		return apperror.Unauthorized("账号已在其他设备登录，请重新登录")
	}
	return nil
}

func (a *Authenticator) latestActiveSession(ctx context.Context, userID int64, platform string) (*Session, *apperror.Error) {
	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	session, err := a.repository.FindLatestActiveByUserPlatform(ctx, userID, platform, a.now())
	if err != nil {
		return nil, apperror.Internal("单端登录会话查询失败")
	}
	return session, nil
}

func (a *Authenticator) deleteCache(ctx context.Context, key string) {
	if a.cache != nil {
		_ = a.cache.Del(ctx, key)
	}
}

func (a *Authenticator) updateSingleSessionPointer(ctx context.Context, session *Session) {
	if a.cache == nil || session == nil {
		return
	}
	_ = a.cache.Set(ctx, a.singleSessionPointerKey(session.Platform, session.UserID), strconv.FormatInt(session.ID, 10), a.cfg.SingleSessionPointerTTL)
}

func (a *Authenticator) clearPointerIfMatches(ctx context.Context, session *Session) {
	if a.cache == nil || session == nil {
		return
	}
	key := a.singleSessionPointerKey(session.Platform, session.UserID)
	current, err := a.cache.Get(ctx, key)
	if err == nil && sameSessionID(current, session.ID) {
		_ = a.cache.Del(ctx, key)
	}
}

func (a *Authenticator) sessionCacheKey(sessionID int64) string {
	return a.cfg.RedisPrefix + "session:" + strconv.FormatInt(sessionID, 10)
}

func (a *Authenticator) singleSessionPointerKey(platform string, userID int64) string {
	return a.cfg.RedisPrefix + "cur_sess:" + strings.ToLower(strings.TrimSpace(platform)) + ":" + strconv.FormatInt(userID, 10)
}

func sameSessionID(value string, sessionID int64) bool {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed == sessionID
}

func identityFromSession(session *Session) *Identity {
	return &Identity{
		UserID:    session.UserID,
		SessionID: session.ID,
		Platform:  session.Platform,
	}
}

func (a *Authenticator) issueAccessToken(sessionID int64, userID int64, platform string, deviceID string, policy *AuthPolicy, now time.Time) (string, string, time.Time, *apperror.Error) {
	if a.accessCodec == nil {
		return "", "", time.Time{}, apperror.Unauthorized("Token认证未配置")
	}
	expiresAt := now.Add(policy.AccessTTL)
	accessToken, err := a.accessCodec.Issue(accesstoken.Claims{
		SessionID: sessionID,
		UserID:    userID,
		Platform:  platform,
		DeviceID:  deviceID,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", "", time.Time{}, apperror.Internal("访问令牌生成失败")
	}
	accessHash, err := HashToken(accessToken, a.tokenPepper)
	if err != nil {
		return "", "", time.Time{}, apperror.Unauthorized("令牌格式错误")
	}
	return accessToken, accessHash, expiresAt, nil
}

func (a *Authenticator) issueRefreshToken(policy *AuthPolicy, now time.Time) (string, string, time.Time, *apperror.Error) {
	refreshToken, err := a.tokenGenerator(64)
	if err != nil {
		return "", "", time.Time{}, apperror.Internal("刷新令牌生成失败")
	}
	refreshHash, err := HashToken(refreshToken, a.tokenPepper)
	if err != nil {
		return "", "", time.Time{}, apperror.Unauthorized("令牌格式错误")
	}
	return refreshToken, refreshHash, now.Add(policy.RefreshTTL), nil
}

func matchClaims(session *Session, claims accesstoken.Claims) *apperror.Error {
	if session == nil {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if session.ID != claims.SessionID || session.UserID != claims.UserID {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if claims.Platform != "" && !strings.EqualFold(session.Platform, claims.Platform) {
		return apperror.Unauthorized("平台不匹配")
	}
	if claims.DeviceID != "" && session.DeviceID != "" && claims.DeviceID != session.DeviceID {
		return apperror.Unauthorized("设备变更，请重新登录")
	}
	return nil
}

func sessionIDPlaceholder(userID int64, platform string, now time.Time) string {
	return strconv.FormatInt(userID, 10) + "|" + platform + "|" + strconv.FormatInt(now.UnixNano(), 10)
}

func temporaryAccessTokenHash(seed string) string {
	sum := sha256.Sum256([]byte("pending|" + seed))
	return hex.EncodeToString(sum[:])
}

func makeToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// Session admin management merged from usersession dto.go
const (
	SessionStatusActive  = "active"
	SessionStatusExpired = "expired"
	SessionStatusRevoked = "revoked"
)

type SessionOption[T string] struct {
	Label string `json:"label"`
	Value T      `json:"value"`
}

type SessionPageInitResponse struct {
	Dict SessionPageInitDict `json:"dict"`
}

type SessionPageInitDict struct {
	PlatformArr []SessionOption[string] `json:"platformArr"`
	StatusArr   []SessionOption[string] `json:"statusArr"`
}

type SessionListQuery struct {
	CurrentPage int
	PageSize    int
	Username    string
	Platform    string
	Status      string
	Now         time.Time
}

type SessionPage struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type SessionListResponse struct {
	List []SessionListItem `json:"list"`
	Page SessionPage       `json:"page"`
}

type SessionListItem struct {
	ID               int64   `json:"id"`
	UserID           int64   `json:"user_id"`
	Username         string  `json:"username"`
	Platform         string  `json:"platform"`
	PlatformName     string  `json:"platform_name"`
	DeviceID         string  `json:"device_id"`
	IP               string  `json:"ip"`
	UserAgent        string  `json:"ua"`
	LastSeenAt       string  `json:"last_seen_at"`
	CreatedAt        string  `json:"created_at"`
	ExpiresAt        string  `json:"expires_at"`
	RefreshExpiresAt string  `json:"refresh_expires_at"`
	RevokedAt        *string `json:"revoked_at"`
	Status           string  `json:"status"`
}

type SessionListRow struct {
	ID               int64
	UserID           int64
	Username         string
	Platform         string
	DeviceID         string
	IP               string
	UserAgent        string
	LastSeenAt       time.Time
	CreatedAt        time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	RevokedAt        *time.Time
}

type SessionStatsResponse struct {
	TotalActive          int64            `json:"total_active"`
	PlatformDistribution map[string]int64 `json:"platform_distribution"`
}

type SessionStatsRow struct {
	Platform string
	Total    int64
}

type SessionRevokeResponse struct {
	ID      int64 `json:"id"`
	Revoked bool  `json:"revoked"`
}

type SessionBatchRevokeInput struct {
	IDs []int64
}

type SessionBatchRevokeResponse struct {
	Count                 int64 `json:"count"`
	SkippedCurrent        int   `json:"skipped_current"`
	SkippedAlreadyRevoked int   `json:"skipped_already_revoked"`
}

type SessionAdminRecord struct {
	ID              int64
	UserID          int64
	Platform        string
	AccessTokenHash string
	RevokedAt       *time.Time
}

// Session admin management merged from usersession repository.go
var ErrSessionAdminRepositoryNotConfigured = errors.New("user session repository is not configured")

type SessionAdminRepository interface {
	List(ctx context.Context, query SessionListQuery) ([]SessionListRow, int64, error)
	Stats(ctx context.Context, now time.Time) ([]SessionStatsRow, error)
	GetByID(ctx context.Context, id int64) (*SessionAdminRecord, error)
	GetByIDs(ctx context.Context, ids []int64) ([]SessionAdminRecord, error)
	MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error)
}

type SessionAdminGormRepository struct {
	db *gorm.DB
}

func NewSessionAdminGormRepository(client *database.Client) *SessionAdminGormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &SessionAdminGormRepository{db: client.Gorm}
}

func (r *SessionAdminGormRepository) List(ctx context.Context, query SessionListQuery) ([]SessionListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrSessionAdminRepositoryNotConfigured
	}
	query.Now = normalizeSessionAdminNow(query.Now)
	db := r.baseSessionListQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []SessionListRow
	err := db.Select(`
			us.id,
			us.user_id,
			COALESCE(u.username, '') AS username,
			us.platform,
			us.device_id,
			us.ip,
			COALESCE(us.ua, '') AS user_agent,
			us.last_seen_at,
			us.created_at,
			us.expires_at,
			us.refresh_expires_at,
			us.revoked_at
		`).
		Order(clause.Expr{
			SQL: `CASE
				WHEN us.revoked_at IS NULL AND us.refresh_expires_at > ? THEN 1
				WHEN us.revoked_at IS NULL AND us.refresh_expires_at <= ? THEN 2
				ELSE 3
			END ASC`,
			Vars:               []any{query.Now, query.Now},
			WithoutParentheses: true,
		}).
		Order("us.last_seen_at DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *SessionAdminGormRepository) Stats(ctx context.Context, now time.Time) ([]SessionStatsRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionAdminRepositoryNotConfigured
	}
	now = normalizeSessionAdminNow(now)
	var rows []SessionStatsRow
	err := r.db.WithContext(ctx).
		Table("user_sessions AS us").
		Where("us.is_del = ?", enum.CommonNo).
		Where("us.revoked_at IS NULL").
		Where("us.refresh_expires_at > ?", now).
		Select("us.platform, COUNT(*) AS total").
		Group("us.platform").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SessionAdminGormRepository) GetByID(ctx context.Context, id int64) (*SessionAdminRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionAdminRepositoryNotConfigured
	}
	var row SessionAdminRecord
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("id, user_id, platform, access_token_hash, revoked_at").
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SessionAdminGormRepository) GetByIDs(ctx context.Context, ids []int64) ([]SessionAdminRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionAdminRepositoryNotConfigured
	}
	if len(ids) == 0 {
		return []SessionAdminRecord{}, nil
	}
	var rows []SessionAdminRecord
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("id, user_id, platform, access_token_hash, revoked_at").
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Order("id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SessionAdminGormRepository) MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrSessionAdminRepositoryNotConfigured
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&SessionAdminRecordModel{}).
		Where("id IN ?", ids).
		Where("revoked_at IS NULL").
		Where("is_del = ?", enum.CommonNo).
		Update("revoked_at", revokedAt)
	return result.RowsAffected, result.Error
}

func (r *SessionAdminGormRepository) baseSessionListQuery(ctx context.Context, query SessionListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("user_sessions AS us").
		Joins("LEFT JOIN users AS u ON u.id = us.user_id").
		Where("us.is_del = ?", enum.CommonNo)

	if query.Username != "" {
		db = db.Where("u.username LIKE ?", strings.TrimSpace(query.Username)+"%")
	}
	if query.Platform != "" {
		db = db.Where("us.platform = ?", query.Platform)
	}
	if query.Status != "" {
		switch query.Status {
		case SessionStatusActive:
			db = db.Where("us.revoked_at IS NULL").Where("us.refresh_expires_at > ?", query.Now)
		case SessionStatusExpired:
			db = db.Where("us.revoked_at IS NULL").Where("us.refresh_expires_at <= ?", query.Now)
		case SessionStatusRevoked:
			db = db.Where("us.revoked_at IS NOT NULL")
		}
	}
	return db
}

func normalizeSessionAdminNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}

type SessionAdminRecordModel struct {
	ID        int64      `gorm:"column:id"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	IsDel     int        `gorm:"column:is_del"`
}

func (SessionAdminRecordModel) TableName() string {
	return "user_sessions"
}

// Session admin management merged from usersession service.go
const sessionAdminTimeLayout = "2006-01-02 15:04:05"

type SessionAdminHTTPService interface {
	PageInit(ctx context.Context) (*SessionPageInitResponse, *apperror.Error)
	List(ctx context.Context, query SessionListQuery) (*SessionListResponse, *apperror.Error)
	Stats(ctx context.Context) (*SessionStatsResponse, *apperror.Error)
	Revoke(ctx context.Context, id int64, currentSessionID int64) (*SessionRevokeResponse, *apperror.Error)
	BatchRevoke(ctx context.Context, input SessionBatchRevokeInput, currentSessionID int64) (*SessionBatchRevokeResponse, *apperror.Error)
}

type SessionAdminOption func(*SessionAdminService)

type SessionAdminCacheRevoker interface {
	RevokeCache(ctx context.Context, row Session) error
	RevokeCaches(ctx context.Context, rows []Session) error
}

type SessionAdminService struct {
	repository   SessionAdminRepository
	cacheRevoker SessionAdminCacheRevoker
	now          func() time.Time
}

func NewSessionAdminService(repository SessionAdminRepository, opts ...SessionAdminOption) *SessionAdminService {
	service := &SessionAdminService{
		repository: repository,
		now:        time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func WithSessionAdminNow(now func() time.Time) SessionAdminOption {
	return func(s *SessionAdminService) {
		if now != nil {
			s.now = now
		}
	}
}

func WithSessionAdminCacheRevoker(revoker SessionAdminCacheRevoker) SessionAdminOption {
	return func(s *SessionAdminService) {
		s.cacheRevoker = revoker
	}
}

func (s *SessionAdminService) PageInit(ctx context.Context) (*SessionPageInitResponse, *apperror.Error) {
	return &SessionPageInitResponse{Dict: SessionPageInitDict{
		PlatformArr: []SessionOption[string]{
			{Label: enum.PlatformAdmin, Value: enum.PlatformAdmin},
			{Label: enum.PlatformApp, Value: enum.PlatformApp},
		},
		StatusArr: []SessionOption[string]{
			{Label: "在线", Value: SessionStatusActive},
			{Label: "已过期", Value: SessionStatusExpired},
			{Label: "已下线", Value: SessionStatusRevoked},
		},
	}}, nil
}

func (s *SessionAdminService) List(ctx context.Context, query SessionListQuery) (*SessionListResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = s.normalizeSessionListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}
	list := make([]SessionListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row, query.Now))
	}
	return &SessionListResponse{
		List: list,
		Page: SessionPage{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *SessionAdminService) Stats(ctx context.Context) (*SessionStatsResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.Stats(ctx, s.now())
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.stats_failed", nil, "查询用户会话统计失败", err)
	}
	dist := map[string]int64{
		enum.PlatformAdmin: 0,
		enum.PlatformApp:   0,
	}
	var total int64
	for _, row := range rows {
		if row.Platform == "" {
			continue
		}
		dist[row.Platform] = row.Total
		total += row.Total
	}
	return &SessionStatsResponse{TotalActive: total, PlatformDistribution: dist}, nil
}

func (s *SessionAdminService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*SessionRevokeResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
	if appErr != nil {
		return nil, appErr
	}
	if id <= 0 {
		return nil, apperror.BadRequestKey("usersession.id.invalid", nil, "无效的用户会话ID")
	}
	if id == currentSessionID {
		return nil, apperror.BadRequestKey("usersession.revoke_current_forbidden", nil, "不能踢下线当前会话")
	}
	row, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}
	if row == nil {
		return nil, apperror.NotFoundKey("usersession.not_found", nil, "用户会话不存在")
	}
	if row.RevokedAt != nil {
		return &SessionRevokeResponse{ID: id, Revoked: false}, nil
	}
	now := s.now()
	if _, err := repo.MarkRevoked(ctx, []int64{id}, now); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.revoke_failed", nil, "踢下线用户会话失败", err)
	}
	if err := s.revokeCache(ctx, *row); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.cache_revoke_failed", nil, "清理用户会话缓存失败", err)
	}
	return &SessionRevokeResponse{ID: id, Revoked: true}, nil
}

func (s *SessionAdminService) BatchRevoke(ctx context.Context, input SessionBatchRevokeInput, currentSessionID int64) (*SessionBatchRevokeResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
	if appErr != nil {
		return nil, appErr
	}
	ids := normalizeIDs(input.IDs)
	if len(ids) > 100 {
		return nil, apperror.BadRequestKey("usersession.batch_too_many", nil, "单次最多踢下线100个会话")
	}
	if len(ids) == 0 {
		return &SessionBatchRevokeResponse{}, nil
	}
	rows, err := repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}

	response := &SessionBatchRevokeResponse{}
	toRevoke := make([]SessionAdminRecord, 0, len(rows))
	revokeIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ID == currentSessionID {
			response.SkippedCurrent++
			continue
		}
		if row.RevokedAt != nil {
			response.SkippedAlreadyRevoked++
			continue
		}
		toRevoke = append(toRevoke, row)
		revokeIDs = append(revokeIDs, row.ID)
	}
	if len(revokeIDs) == 0 {
		return response, nil
	}
	count, err := repo.MarkRevoked(ctx, revokeIDs, s.now())
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.batch_revoke_failed", nil, "批量踢下线用户会话失败", err)
	}
	if err := s.revokeCaches(ctx, toRevoke); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.cache_revoke_failed", nil, "清理用户会话缓存失败", err)
	}
	response.Count = count
	return response, nil
}

func (s *SessionAdminService) requireSessionAdminRepository() (SessionAdminRepository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("usersession.repository_missing", nil, "用户会话仓储未配置")
	}
	return s.repository, nil
}

func (s *SessionAdminService) normalizeSessionListQuery(query SessionListQuery) (SessionListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Username = strings.TrimSpace(query.Username)
	query.Platform = strings.TrimSpace(query.Platform)
	query.Status = strings.TrimSpace(query.Status)
	if query.Platform != "" && !enum.IsPlatform(query.Platform) {
		return query, apperror.BadRequestKey("usersession.platform.invalid", nil, "无效的平台标识")
	}
	if query.Status != "" && !isSessionStatus(query.Status) {
		return query, apperror.BadRequestKey("usersession.status.invalid", nil, "无效的会话状态")
	}
	query.Now = s.now()
	return query, nil
}

func (s *SessionAdminService) revokeCache(ctx context.Context, row SessionAdminRecord) error {
	if s == nil || s.cacheRevoker == nil {
		return nil
	}
	return s.cacheRevoker.RevokeCache(ctx, sessionFromAdminRecord(row))
}

func (s *SessionAdminService) revokeCaches(ctx context.Context, rows []SessionAdminRecord) error {
	if s == nil || s.cacheRevoker == nil {
		return nil
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, sessionFromAdminRecord(row))
	}
	return s.cacheRevoker.RevokeCaches(ctx, sessions)
}

func sessionFromAdminRecord(row SessionAdminRecord) Session {
	return Session{ID: row.ID, UserID: row.UserID, Platform: row.Platform, AccessTokenHash: row.AccessTokenHash}
}

func normalizeIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func isSessionStatus(value string) bool {
	return value == SessionStatusActive || value == SessionStatusExpired || value == SessionStatusRevoked
}

func listItem(row SessionListRow, now time.Time) SessionListItem {
	return SessionListItem{
		ID: row.ID, UserID: row.UserID, Username: row.Username,
		Platform: row.Platform, PlatformName: platformName(row.Platform),
		DeviceID: row.DeviceID, IP: row.IP, UserAgent: row.UserAgent,
		LastSeenAt: formatTime(row.LastSeenAt), CreatedAt: formatTime(row.CreatedAt),
		ExpiresAt: formatTime(row.ExpiresAt), RefreshExpiresAt: formatTime(row.RefreshExpiresAt),
		RevokedAt: formatOptionalTime(row.RevokedAt), Status: sessionStatus(row, now),
	}
}

func sessionStatus(row SessionListRow, now time.Time) string {
	if row.RevokedAt != nil {
		return SessionStatusRevoked
	}
	if !row.RefreshExpiresAt.After(now) {
		return SessionStatusExpired
	}
	return SessionStatusActive
}

func platformName(platform string) string {
	for _, item := range dict.PlatformOptions() {
		if item.Value == platform {
			return item.Label
		}
	}
	return platform
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.Format(sessionAdminTimeLayout)
	return &formatted
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(sessionAdminTimeLayout)
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
