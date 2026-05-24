package permission

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"admin_back_go/internal/platform/redisclient"

	"github.com/redis/go-redis/v9"
)

// RedisRouteAccessGrantCache stores computed RBAC route access grant codes by user and platform.
// The cached value may contain PAGE codes and BUTTON codes; it is not the public users/init buttonCodes contract.
type RedisRouteAccessGrantCache struct {
	client *redisclient.Client
}

// NewRedisRouteAccessGrantCache returns nil when Redis is not configured so callers can keep cache optional.
func NewRedisRouteAccessGrantCache(client *redisclient.Client) *RedisRouteAccessGrantCache {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisRouteAccessGrantCache{client: client}
}

// Get returns cached route access grant codes. The bool is false on cache miss.
func (c *RedisRouteAccessGrantCache) Get(ctx context.Context, key string) ([]string, bool, error) {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return nil, false, nil
	}

	payload, err := c.client.Redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	var values []string
	if err := json.Unmarshal([]byte(payload), &values); err != nil {
		return nil, false, err
	}
	return values, true, nil
}

// Set stores route access grant codes with the supplied TTL.
func (c *RedisRouteAccessGrantCache) Set(ctx context.Context, key string, values []string, ttl time.Duration) error {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return c.client.Redis.Set(ctx, key, payload, ttl).Err()
}

// Delete removes one cached route access grant entry.
func (c *RedisRouteAccessGrantCache) Delete(ctx context.Context, key string) error {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return nil
	}
	return c.client.Redis.Del(ctx, key).Err()
}
