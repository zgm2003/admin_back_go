package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/infra/redisclient"

	"github.com/redis/go-redis/v9"
)

var ErrSessionCacheNotConfigured = errors.New("session cache is not configured")

var ErrUnsupportedSessionCacheSchema = errors.New("unsupported session cache schema")

const SessionCacheSchemaVersion = 1

type CachedSession struct {
	SessionID        int64      `json:"session_id"`
	UserID           int64      `json:"user_id"`
	Platform         string     `json:"platform"`
	DeviceID         string     `json:"device_id"`
	IP               string     `json:"ip"`
	AccessExpiresAt  time.Time  `json:"access_expires_at"`
	RefreshExpiresAt time.Time  `json:"refresh_expires_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	SchemaVersion    int        `json:"schema_version"`
}

func parseCachedSession(value string, loc *time.Location) (*Session, error) {
	_ = loc
	var payload CachedSession
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return nil, fmt.Errorf("decode cached session: %w", err)
	}
	if payload.SchemaVersion != SessionCacheSchemaVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedSessionCacheSchema, payload.SchemaVersion)
	}
	if payload.SessionID <= 0 || payload.UserID <= 0 || strings.TrimSpace(payload.Platform) == "" || payload.AccessExpiresAt.IsZero() {
		return nil, errors.New("invalid cached session")
	}
	return &Session{
		ID:               payload.SessionID,
		UserID:           payload.UserID,
		Platform:         payload.Platform,
		DeviceID:         payload.DeviceID,
		IP:               payload.IP,
		ExpiresAt:        payload.AccessExpiresAt,
		RefreshExpiresAt: payload.RefreshExpiresAt,
		RevokedAt:        payload.RevokedAt,
		IsDel:            commonNo,
	}, nil
}

func cacheValue(session *Session) string {
	if session == nil {
		return ""
	}
	payload := CachedSession{
		SessionID:        session.ID,
		UserID:           session.UserID,
		Platform:         session.Platform,
		DeviceID:         session.DeviceID,
		IP:               session.IP,
		AccessExpiresAt:  session.ExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
		RevokedAt:        session.RevokedAt,
		SchemaVersion:    SessionCacheSchemaVersion,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
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
