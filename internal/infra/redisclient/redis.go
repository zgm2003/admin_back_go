package redisclient

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"github.com/redis/go-redis/v9"
)

var ErrEmptyAddress = errors.New("redis address is empty")

type Client struct {
	Redis *redis.Client
}

type openOptions struct {
	recorder telemetry.Recorder
}

type Option func(*openOptions)

func WithTelemetry(recorder telemetry.Recorder) Option {
	return func(options *openOptions) {
		options.recorder = recorder
	}
}

func Open(cfg config.RedisConfig, options ...Option) (*Client, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, ErrEmptyAddress
	}
	settings := openOptions{}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if settings.recorder != nil {
		client.AddHook(newTelemetryHook(settings.recorder))
	}
	return &Client{Redis: client}, nil
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
