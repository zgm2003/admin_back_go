package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/shared/apperror"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRealtimeTicketTTL = 30 * time.Second
	defaultQueueGrantTTL     = 60 * time.Second
)

type GrantSubject struct {
	SessionID int64  `json:"session_id"`
	UserID    int64  `json:"user_id"`
	Platform  string `json:"platform"`
}

type BrowserGrant struct {
	Credential string `json:"ticket,omitempty"`
	ExpiresIn  int64  `json:"expires_in"`
}

type BrowserGrantStore interface {
	Put(context.Context, string, string, time.Duration) error
	Consume(context.Context, string) (string, error)
	Get(context.Context, string) (string, error)
}

type BrowserGrantConfig struct {
	RedisPrefix    string
	RealtimeTTL    time.Duration
	QueueTTL       time.Duration
	TokenGenerator TokenGenerator
}

type BrowserGrantService struct {
	store          BrowserGrantStore
	prefix         string
	realtimeTTL    time.Duration
	queueTTL       time.Duration
	tokenGenerator TokenGenerator
}

func NewBrowserGrantService(store BrowserGrantStore, cfg BrowserGrantConfig) *BrowserGrantService {
	prefix := strings.TrimSpace(cfg.RedisPrefix)
	if prefix == "" {
		prefix = "token:"
	}
	if cfg.RealtimeTTL <= 0 {
		cfg.RealtimeTTL = defaultRealtimeTicketTTL
	}
	if cfg.QueueTTL <= 0 {
		cfg.QueueTTL = defaultQueueGrantTTL
	}
	if cfg.TokenGenerator == nil {
		cfg.TokenGenerator = makeToken
	}
	return &BrowserGrantService{
		store:          store,
		prefix:         prefix,
		realtimeTTL:    cfg.RealtimeTTL,
		queueTTL:       cfg.QueueTTL,
		tokenGenerator: cfg.TokenGenerator,
	}
}

func (s *BrowserGrantService) IssueRealtimeTicket(ctx context.Context, subject GrantSubject) (*BrowserGrant, *apperror.Error) {
	return s.issue(ctx, "realtime", subject, s.realtimeTTL)
}

func (s *BrowserGrantService) ConsumeRealtimeTicket(ctx context.Context, credential string) (GrantSubject, *apperror.Error) {
	return s.read(ctx, "realtime", credential, true)
}

func (s *BrowserGrantService) IssueQueueMonitorGrant(ctx context.Context, subject GrantSubject) (*BrowserGrant, *apperror.Error) {
	return s.issue(ctx, "queue-monitor", subject, s.queueTTL)
}

func (s *BrowserGrantService) ValidateQueueMonitorGrant(ctx context.Context, credential string) (GrantSubject, *apperror.Error) {
	return s.read(ctx, "queue-monitor", credential, false)
}

func (s *BrowserGrantService) issue(ctx context.Context, kind string, subject GrantSubject, ttl time.Duration) (*BrowserGrant, *apperror.Error) {
	if s == nil || s.store == nil || subject.SessionID <= 0 || subject.UserID <= 0 || strings.TrimSpace(subject.Platform) == "" {
		return nil, browserGrantInternal(errors.New("browser grant service or subject is invalid"))
	}
	credential, err := s.tokenGenerator(32)
	if err != nil {
		return nil, browserGrantInternal(err)
	}
	payload, err := json.Marshal(subject)
	if err != nil {
		return nil, browserGrantInternal(err)
	}
	if err := s.store.Put(ctx, s.key(kind, credential), string(payload), ttl); err != nil {
		return nil, browserGrantInternal(err)
	}
	return &BrowserGrant{Credential: credential, ExpiresIn: int64(ttl.Seconds())}, nil
}

func (s *BrowserGrantService) read(ctx context.Context, kind string, credential string, consume bool) (GrantSubject, *apperror.Error) {
	if s == nil || s.store == nil || strings.TrimSpace(credential) == "" {
		return GrantSubject{}, invalidBrowserGrant()
	}
	var (
		payload string
		err     error
	)
	if consume {
		payload, err = s.store.Consume(ctx, s.key(kind, credential))
	} else {
		payload, err = s.store.Get(ctx, s.key(kind, credential))
	}
	if err != nil {
		return GrantSubject{}, browserGrantInternal(err)
	}
	if payload == "" {
		return GrantSubject{}, invalidBrowserGrant()
	}
	var subject GrantSubject
	if err := json.Unmarshal([]byte(payload), &subject); err != nil || subject.SessionID <= 0 || subject.UserID <= 0 || strings.TrimSpace(subject.Platform) == "" {
		return GrantSubject{}, invalidBrowserGrant()
	}
	return subject, nil
}

func (s *BrowserGrantService) key(kind string, credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return s.prefix + "browser-grant:" + kind + ":" + hex.EncodeToString(sum[:])
}

func invalidBrowserGrant() *apperror.Error {
	return apperror.New("auth.browser_grant_invalid", apperror.CategoryAuthentication, 0, apperror.Permanent, "auth.browser_grant_invalid", nil, "浏览器授权已失效")
}

func browserGrantInternal(cause error) *apperror.Error {
	return apperror.Wrap("auth.browser_grant_unavailable", apperror.CategoryDependency, 0, apperror.Retryable, "auth.browser_grant_unavailable", nil, "浏览器授权服务不可用", cause)
}

type RedisBrowserGrantStore struct {
	client *redis.Client
}

func NewRedisBrowserGrantStore(client *redisclient.Client) BrowserGrantStore {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisBrowserGrantStore{client: client.Redis}
}

func (s *RedisBrowserGrantStore) Put(ctx context.Context, key string, value string, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return ErrSessionCacheNotConfigured
	}
	stored, err := s.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return err
	}
	if !stored {
		return errors.New("browser grant collision")
	}
	return nil
}

var consumeBrowserGrantScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if value then
  redis.call('DEL', KEYS[1])
  return value
end
return ''
`)

func (s *RedisBrowserGrantStore) Consume(ctx context.Context, key string) (string, error) {
	if s == nil || s.client == nil {
		return "", ErrSessionCacheNotConfigured
	}
	return consumeBrowserGrantScript.Run(ctx, s.client, []string{key}).Text()
}

func (s *RedisBrowserGrantStore) Get(ctx context.Context, key string) (string, error) {
	if s == nil || s.client == nil {
		return "", ErrSessionCacheNotConfigured
	}
	value, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return value, err
}
