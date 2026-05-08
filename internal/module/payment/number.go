package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const paymentOrderNoCounterKey = "payment_order_no_counter"

type Counter interface {
	Incr(ctx context.Context, key string) (int64, error)
}

type redisCounter struct {
	client redis.Cmdable
}

func (c redisCounter) Incr(ctx context.Context, key string) (int64, error) {
	if c.client == nil {
		return 0, errors.New("payment number: redis client not configured")
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
	if prefix != "P" {
		return "", fmt.Errorf("payment number: invalid prefix %q", prefix)
	}
	if g == nil || g.counter == nil {
		return "", errors.New("payment number: counter not configured")
	}
	seq, err := g.counter.Incr(ctx, paymentOrderNoCounterKey)
	if err != nil {
		return "", fmt.Errorf("payment number: incr: %w", err)
	}
	return fmt.Sprintf("%s%s%06d", prefix, g.now().Format("060102150405"), seq%1000000), nil
}
