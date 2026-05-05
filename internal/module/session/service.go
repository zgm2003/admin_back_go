package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
)

var (
	ErrCacheNotConfigured      = errors.New("session cache is not configured")
	ErrRepositoryNotConfigured = errors.New("session repository is not configured")
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
	Cache          Cache
	Repository     Repository
	PolicyProvider PolicyProvider
	TokenGenerator TokenGenerator
	Now            func() time.Time
}

type Authenticator struct {
	cfg            config.TokenConfig
	cache          Cache
	repository     Repository
	policyProvider PolicyProvider
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
	tokens, tokenErr := a.generateTokenPair(policy, now)
	if tokenErr != nil {
		return nil, tokenErr
	}

	if evictErr := a.evictSessions(ctx, input.UserID, input.Platform, policy, now); evictErr != nil {
		return nil, evictErr
	}

	sessionID, err := a.repository.Create(ctx, SessionCreate{
		UserID:           input.UserID,
		AccessTokenHash:  tokens.AccessTokenHash,
		RefreshTokenHash: tokens.RefreshTokenHash,
		Platform:         input.Platform,
		DeviceID:         input.DeviceID,
		IP:               input.ClientIP,
		UserAgent:        input.UserAgent,
		LastSeenAt:       now,
		ExpiresAt:        tokens.AccessExpiresAt,
		RefreshExpiresAt: tokens.RefreshExpiresAt,
	})
	if err != nil {
		return nil, apperror.Internal("创建登录会话失败")
	}

	a.updateSingleSessionPointer(ctx, &Session{ID: sessionID, UserID: input.UserID, Platform: input.Platform})
	return &TokenResult{
		AccessToken:      tokens.AccessToken,
		RefreshToken:     tokens.RefreshToken,
		ExpiresIn:        int(tokens.AccessTTL.Seconds()),
		RefreshExpiresIn: int(tokens.RefreshTTL.Seconds()),
	}, nil
}

func NewAuthenticator(deps AuthenticatorDeps) *Authenticator {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	if deps.Config.RedisPrefix == "" {
		deps.Config.RedisPrefix = "token:"
	}
	if deps.Config.SessionCacheTTL == 0 {
		deps.Config.SessionCacheTTL = 30 * time.Minute
	}
	if deps.Config.SingleSessionPointerTTL == 0 {
		deps.Config.SingleSessionPointerTTL = 30 * 24 * time.Hour
	}
	tokenGenerator := deps.TokenGenerator
	if tokenGenerator == nil {
		tokenGenerator = makeToken
	}
	return &Authenticator{
		cfg:            deps.Config,
		cache:          deps.Cache,
		repository:     deps.Repository,
		policyProvider: deps.PolicyProvider,
		tokenGenerator: tokenGenerator,
		now:            now,
		loc:            time.Local,
	}
}

func (a *Authenticator) Authenticate(ctx context.Context, input TokenInput) (*Identity, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	tokenHash, err := HashToken(input.AccessToken, a.cfg.Pepper)
	if err != nil {
		return nil, apperror.Unauthorized("Token格式错误")
	}

	cacheKey := a.cacheKey(tokenHash)
	if a.cache != nil {
		if session, cacheErr := a.sessionFromCache(ctx, cacheKey); cacheErr != nil {
			return nil, cacheErr
		} else if session != nil {
			if policyErr := a.enforcePolicy(ctx, cacheKey, session, input); policyErr != nil {
				return nil, policyErr
			}
			return identityFromSession(session), nil
		}
	}

	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	session, repoErr := a.repository.FindValidByAccessHash(ctx, tokenHash, a.now())
	if repoErr != nil {
		return nil, apperror.Internal("Token会话查询失败")
	}
	if session == nil {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if !session.ExpiresAt.After(a.now()) {
		return nil, apperror.Unauthorized("Token已过期")
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

	refreshHash, err := HashToken(input.RefreshToken, a.cfg.Pepper)
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

	tokens, tokenErr := a.generateTokenPair(policy, now)
	if tokenErr != nil {
		return nil, tokenErr
	}

	rotation := Rotation{
		AccessTokenHash:  tokens.AccessTokenHash,
		RefreshTokenHash: tokens.RefreshTokenHash,
		ExpiresAt:        tokens.AccessExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
		LastSeenAt:       now,
		IP:               input.ClientIP,
		UserAgent:        input.UserAgent,
	}
	if err := a.repository.Rotate(ctx, session.ID, rotation); err != nil {
		return nil, apperror.Internal("刷新令牌更新失败")
	}

	if session.AccessTokenHash != "" {
		a.deleteCache(ctx, a.cacheKey(session.AccessTokenHash))
	}
	a.updateSingleSessionPointer(ctx, session)

	return &TokenResult{
		AccessToken:      tokens.AccessToken,
		RefreshToken:     tokens.RefreshToken,
		ExpiresIn:        int(tokens.AccessTTL.Seconds()),
		RefreshExpiresIn: int(tokens.RefreshTTL.Seconds()),
	}, nil
}

func (a *Authenticator) Logout(ctx context.Context, accessToken string) *apperror.Error {
	if a == nil || strings.TrimSpace(accessToken) == "" {
		return nil
	}

	accessHash, err := HashToken(accessToken, a.cfg.Pepper)
	if err != nil {
		return nil
	}
	if a.repository == nil {
		return nil
	}

	session, err := a.repository.FindValidByAccessHash(ctx, accessHash, a.now())
	if err != nil || session == nil {
		return nil
	}
	if err := a.repository.Revoke(ctx, session.ID, a.now()); err != nil {
		return apperror.Internal("退出登录失败")
	}

	a.deleteCache(ctx, a.cacheKey(accessHash))
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
		a.deleteCache(ctx, a.cacheKey(oldSession.AccessTokenHash))
	}
	return nil
}

func (a *Authenticator) deleteSessionCaches(ctx context.Context, sessions []Session) {
	for _, session := range sessions {
		if session.AccessTokenHash != "" {
			a.deleteCache(ctx, a.cacheKey(session.AccessTokenHash))
		}
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

func (a *Authenticator) cacheKey(tokenHash string) string {
	return a.cfg.RedisPrefix + tokenHash
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

type tokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessTokenHash  string
	RefreshTokenHash string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
}

func (a *Authenticator) generateTokenPair(policy *AuthPolicy, now time.Time) (*tokenPair, *apperror.Error) {
	accessToken, err := a.tokenGenerator(32)
	if err != nil {
		return nil, apperror.Internal("访问令牌生成失败")
	}
	refreshToken, err := a.tokenGenerator(64)
	if err != nil {
		return nil, apperror.Internal("刷新令牌生成失败")
	}
	accessHash, err := HashToken(accessToken, a.cfg.Pepper)
	if err != nil {
		return nil, apperror.Unauthorized("令牌格式错误")
	}
	refreshHash, err := HashToken(refreshToken, a.cfg.Pepper)
	if err != nil {
		return nil, apperror.Unauthorized("令牌格式错误")
	}
	return &tokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessTokenHash:  accessHash,
		RefreshTokenHash: refreshHash,
		AccessExpiresAt:  now.Add(policy.AccessTTL),
		RefreshExpiresAt: now.Add(policy.RefreshTTL),
		AccessTTL:        policy.AccessTTL,
		RefreshTTL:       policy.RefreshTTL,
	}, nil
}

func makeToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
