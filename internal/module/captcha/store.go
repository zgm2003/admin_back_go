package captcha

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"admin_back_go/internal/platform/redisclient"

	"github.com/redis/go-redis/v9"
)

const defaultRedisPrefix = "captcha:slide:"

// ChallengeSecret is the server-side answer stored with a short TTL.
type ChallengeSecret struct {
	Answer Answer `json:"answer"`
}

// Store persists and consumes CAPTCHA answers.
type Store interface {
	Set(ctx context.Context, id string, secret ChallengeSecret, ttl time.Duration) error
	Take(ctx context.Context, id string) (*ChallengeSecret, error)
}

// RedisStore stores CAPTCHA answers in Redis and consumes them via GETDEL.
type RedisStore struct {
	client *redis.Client
	prefix string
}

// NewRedisStore returns nil when Redis is not configured.
func NewRedisStore(client *redisclient.Client, prefix string) *RedisStore {
	if client == nil || client.Redis == nil {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = defaultRedisPrefix
	}
	return &RedisStore{client: client.Redis, prefix: prefix}
}

// Set stores one challenge answer with the supplied TTL.
func (s *RedisStore) Set(ctx context.Context, id string, secret ChallengeSecret, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return ErrStoreNotConfigured
	}
	payload, err := json.Marshal(secret)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(id), payload, ttl).Err()
}

// Take atomically reads and deletes one challenge answer.
func (s *RedisStore) Take(ctx context.Context, id string) (*ChallengeSecret, error) {
	if s == nil || s.client == nil {
		return nil, ErrStoreNotConfigured
	}
	payload, err := s.client.GetDel(ctx, s.key(id)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var secret ChallengeSecret
	if err := json.Unmarshal([]byte(payload), &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

func (s *RedisStore) key(id string) string {
	return s.prefix + strings.TrimSpace(id)
}
