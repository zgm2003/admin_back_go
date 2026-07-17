package redisclient

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/config"

	"github.com/redis/go-redis/v9"
)

var ErrEmptyAddress = errors.New("redis address is empty")

type Client struct {
	Redis *redis.Client
}

func Open(cfg config.RedisConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, ErrEmptyAddress
	}
	return &Client{
		Redis: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}, nil
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
