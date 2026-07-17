package redislock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLeaseAcquireRenewReleaseUsesOwnerAndMonotonicFencingToken(t *testing.T) {
	client := redisLeaseTestClient(t)
	store := New(client)
	ctx := context.Background()
	key := fmt.Sprintf("test:redislock:%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = client.Del(ctx, key, fencingKey(key)).Err()
	})

	first, err := store.Acquire(ctx, key, "worker-a", 150*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire first lease: %v", err)
	}
	if first.Key != key || first.Owner != "worker-a" || first.Token == 0 || first.ExpiresAt.IsZero() {
		t.Fatalf("unexpected first lease: %#v", first)
	}

	if _, err := store.Acquire(ctx, key, "worker-b", time.Second); !errors.Is(err, ErrNotAcquired) {
		t.Fatalf("second owner must not acquire live lease, got %v", err)
	}

	staleOwner := first
	staleOwner.Owner = "worker-b"
	if _, err := store.Renew(ctx, staleOwner, time.Second); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong owner renewed lease, got %v", err)
	}
	staleToken := first
	staleToken.Token++
	if err := store.Release(ctx, staleToken); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong token released lease, got %v", err)
	}

	renewed, err := store.Renew(ctx, first, time.Second)
	if err != nil {
		t.Fatalf("Renew current lease: %v", err)
	}
	if renewed.Token != first.Token || !renewed.ExpiresAt.After(first.ExpiresAt) {
		t.Fatalf("renew must preserve token and extend expiry: first=%#v renewed=%#v", first, renewed)
	}
	if err := store.Release(ctx, renewed); err != nil {
		t.Fatalf("Release current lease: %v", err)
	}

	second, err := store.Acquire(ctx, key, "worker-b", time.Second)
	if err != nil {
		t.Fatalf("Acquire replacement lease: %v", err)
	}
	if second.Token <= first.Token {
		t.Fatalf("fencing token did not increase: first=%d second=%d", first.Token, second.Token)
	}
	if _, err := store.Renew(ctx, first, time.Second); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale lease renewed replacement, got %v", err)
	}
	if err := store.Release(ctx, first); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale lease released replacement, got %v", err)
	}
}

func TestLeaseExpiryAllowsReacquireWithHigherFencingToken(t *testing.T) {
	client := redisLeaseTestClient(t)
	store := New(client)
	ctx := context.Background()
	key := fmt.Sprintf("test:redislock:expiry:%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = client.Del(ctx, key, fencingKey(key)).Err()
	})

	first, err := store.Acquire(ctx, key, "worker-a", 40*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire first lease: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	second, err := store.Acquire(ctx, key, "worker-b", time.Second)
	if err != nil {
		t.Fatalf("Acquire expired lease: %v", err)
	}
	if second.Token <= first.Token {
		t.Fatalf("replacement token must increase: first=%d second=%d", first.Token, second.Token)
	}
}

func TestLeaseStoreRejectsInvalidInputs(t *testing.T) {
	var nilStore *RedisLeaseStore
	if _, err := nilStore.Acquire(context.Background(), "key", "owner", time.Second); err == nil {
		t.Fatal("nil store must reject acquire")
	}

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	store := New(client)
	for _, test := range []struct {
		name  string
		key   string
		owner string
		ttl   time.Duration
	}{
		{name: "empty key", owner: "owner", ttl: time.Second},
		{name: "empty owner", key: "key", ttl: time.Second},
		{name: "zero ttl", key: "key", owner: "owner"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Acquire(context.Background(), test.key, test.owner, test.ttl); err == nil {
				t.Fatal("expected invalid input error")
			}
		})
	}
	if _, err := store.Renew(context.Background(), Lease{}, time.Second); err == nil {
		t.Fatal("invalid lease must not renew")
	}
	if _, err := store.Renew(context.Background(), Lease{Key: "key", Owner: "owner", Token: 1}, 0); err == nil {
		t.Fatal("invalid renewal ttl must be rejected")
	}
	if err := store.Release(context.Background(), Lease{}); err == nil {
		t.Fatal("invalid lease must not release")
	}
}

func redisLeaseTestClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is required for Redis lease integration tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("connect test Redis %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
