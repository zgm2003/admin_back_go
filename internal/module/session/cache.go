package session

import (
	"context"
	"time"

	"admin_back_go/internal/platform/redisclient"

	"github.com/redis/go-redis/v9"
)

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redisclient.Client) Cache {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisCache{client: client.Redis}
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrCacheNotConfigured
	}
	value, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return value, err
}

func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return ErrCacheNotConfigured
	}
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return ErrCacheNotConfigured
	}
	return c.client.Expire(ctx, key, ttl).Err()
}

func (c *RedisCache) Del(ctx context.Context, key string) error {
	if c == nil || c.client == nil {
		return ErrCacheNotConfigured
	}
	return c.client.Del(ctx, key).Err()
}
