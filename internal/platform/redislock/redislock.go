package redislock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotAcquired = errors.New("redislock: lock not acquired")

const unlockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

type Locker interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (token string, err error)
	Unlock(ctx context.Context, key string, token string) error
}

type RedisLocker struct {
	client redis.Cmdable
}

func New(client redis.Cmdable) *RedisLocker {
	return &RedisLocker{client: client}
}

func (l *RedisLocker) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if l == nil || l.client == nil {
		return "", errors.New("redislock: client not configured")
	}
	if key == "" || ttl <= 0 {
		return "", errors.New("redislock: invalid lock input")
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", fmt.Errorf("redislock: setnx: %w", err)
	}
	if !ok {
		return "", ErrNotAcquired
	}
	return token, nil
}

func (l *RedisLocker) Unlock(ctx context.Context, key string, token string) error {
	if l == nil || l.client == nil {
		return errors.New("redislock: client not configured")
	}
	if key == "" || token == "" {
		return errors.New("redislock: invalid unlock input")
	}
	if err := l.client.Eval(ctx, unlockScript, []string{key}, token).Err(); err != nil {
		return fmt.Errorf("redislock: unlock: %w", err)
	}
	return nil
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("redislock: random token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
