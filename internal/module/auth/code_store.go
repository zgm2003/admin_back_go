package auth

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/platform/redisclient"

	"github.com/redis/go-redis/v9"
)

const defaultVerifyCodeRedisPrefix = "auth:verify_code:"

type CodeStore interface {
	Set(ctx context.Context, key string, code string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type RedisCodeStore struct {
	client *redisclient.Client
}

func NewRedisCodeStore(client *redisclient.Client) CodeStore {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisCodeStore{client: client}
}

func (s *RedisCodeStore) Set(ctx context.Context, key string, code string, ttl time.Duration) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	return s.client.Redis.Set(ctx, key, code, ttl).Err()
}

func (s *RedisCodeStore) Get(ctx context.Context, key string) (string, error) {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return "", ErrRepositoryNotConfigured
	}
	value, err := s.client.Redis.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (s *RedisCodeStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	return s.client.Redis.Del(ctx, key).Err()
}

func verifyCodeKey(accountType string, scene string, account string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(account)))
	return accountType + ":" + strings.TrimSpace(scene) + ":" + hex.EncodeToString(sum[:])
}
