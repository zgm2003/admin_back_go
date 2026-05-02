package redisclient

import (
	"context"

	"admin_back_go/internal/config"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	Redis *redis.Client
}

func Open(cfg config.RedisConfig) *Client {
	return &Client{
		Redis: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Redis == nil {
		return redis.ErrClosed
	}
	return c.Redis.Ping(ctx).Err()
}

func (c *Client) Close() error {
	if c == nil || c.Redis == nil {
		return nil
	}
	return c.Redis.Close()
}
