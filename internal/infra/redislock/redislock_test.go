package redislock

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRandomTokenReturnsDistinctHexTokens(t *testing.T) {
	first, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken returned error: %v", err)
	}
	second, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken returned error: %v", err)
	}
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("expected 32-char hex tokens, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("expected distinct tokens, got %q", first)
	}
}

func TestRedisLockerRejectsInvalidLockInput(t *testing.T) {
	locker := New(nil)

	if _, err := locker.Lock(context.Background(), "key", time.Second); err == nil {
		t.Fatalf("expected nil client error")
	}

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	locker = New(client)
	if _, err := locker.Lock(context.Background(), "", time.Second); err == nil {
		t.Fatalf("expected empty key error")
	}
	if _, err := locker.Lock(context.Background(), "key", 0); err == nil {
		t.Fatalf("expected ttl error")
	}
}

func TestRedisLockerRejectsInvalidUnlockInput(t *testing.T) {
	locker := New(nil)
	if err := locker.Unlock(context.Background(), "key", "token"); err == nil {
		t.Fatalf("expected nil client error")
	}

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	locker = New(client)
	if err := locker.Unlock(context.Background(), "", "token"); err == nil {
		t.Fatalf("expected empty key error")
	}
	if err := locker.Unlock(context.Background(), "key", ""); err == nil {
		t.Fatalf("expected empty token error")
	}
}
