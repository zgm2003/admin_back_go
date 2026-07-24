package redeemcode

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

var integrationUserSequence atomic.Int64

func TestRedisAttemptLimiterIntegration(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR"))
	if address == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}
	client := redis.NewClient(&redis.Options{Addr: address, DB: 14})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	userID := time.Now().UnixNano()
	if userID <= 0 {
		userID = integrationUserSequence.Add(1)
	}
	limiter := NewRedisAttemptLimiter(client)
	attemptKey, failureKey, err := limiter.keys("admin", userID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), attemptKey, failureKey).Err() })

	lease, err := limiter.Acquire(ctx, "admin", userID)
	if err != nil {
		t.Fatal(err)
	}
	pTTL, err := client.PTTL(ctx, attemptKey).Result()
	if err != nil || pTTL <= 0 || pTTL > attemptLockTTL {
		t.Fatalf("attempt ttl=%v err=%v", pTTL, err)
	}
	if _, err := limiter.Acquire(ctx, "admin", userID); err == nil {
		t.Fatal("second acquire unexpectedly succeeded")
	} else if locked, ok := err.(*AttemptLockedError); !ok || locked.RetryAfter < 1 {
		t.Fatalf("second acquire error=%T %v", err, err)
	}

	state, err := limiter.RecordFailure(ctx, "admin", userID)
	if err != nil || state.Count != 1 || state.TTL <= 0 || state.TTL > failureWindow {
		t.Fatalf("failure state=%+v err=%v", state, err)
	}
	if _, err := limiter.FailureState(ctx, "admin", userID); err != nil {
		t.Fatal(err)
	}
	if err := limiter.Release(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if exists, err := client.Exists(ctx, attemptKey).Result(); err != nil || exists != 0 {
		t.Fatalf("attempt key exists=%d err=%v", exists, err)
	}
	if exists, err := client.Exists(ctx, attemptKey+":fencing-token").Result(); err != nil || exists != 0 {
		t.Fatalf("unexpected fencing key exists=%d err=%v", exists, err)
	}
}
