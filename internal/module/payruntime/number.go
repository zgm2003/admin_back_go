package payruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const orderNoCounterKey = "pay_order_no_counter"

type Counter interface {
	Incr(ctx context.Context, key string) (int64, error)
}

type redisCounter struct {
	client redis.Cmdable
}

func (c redisCounter) Incr(ctx context.Context, key string) (int64, error) {
	if c.client == nil {
		return 0, errors.New("pay runtime number: redis client not configured")
	}
	return c.client.Incr(ctx, key).Result()
}

type RedisNumberGenerator struct {
	counter Counter
	now     func() time.Time
}

func NewRedisNumberGenerator(counter Counter, now func() time.Time) *RedisNumberGenerator {
	if now == nil {
		now = time.Now
	}
	return &RedisNumberGenerator{counter: counter, now: now}
}

func NewRedisNumberGeneratorFromRedis(client redis.Cmdable) *RedisNumberGenerator {
	return NewRedisNumberGenerator(redisCounter{client: client}, time.Now)
}

func (g *RedisNumberGenerator) Next(ctx context.Context, prefix string) (string, error) {
	if prefix != "R" && prefix != "T" && prefix != "D" {
		return "", fmt.Errorf("pay runtime number: invalid prefix %q", prefix)
	}
	if g == nil || g.counter == nil {
		return "", errors.New("pay runtime number: counter not configured")
	}
	seq, err := g.counter.Incr(ctx, orderNoCounterKey)
	if err != nil {
		return "", fmt.Errorf("pay runtime number: incr: %w", err)
	}
	return fmt.Sprintf("%s%s%06d", prefix, g.now().Format("060102150405"), seq%1000000), nil
}
