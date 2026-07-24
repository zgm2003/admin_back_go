package redeemcode

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLimiterRateLimitContract(t *testing.T) {
	if redeemRedisPrefix != "admin_go:wallet:redeem:v1:" {
		t.Fatalf("prefix=%q", redeemRedisPrefix)
	}
	if failureLimit != 10 || failureWindow != 10*time.Minute || attemptLockTTL != 15*time.Second || attemptTimeout != 10*time.Second || cleanupTimeout != 2*time.Second {
		t.Fatalf("unexpected limiter durations and limit")
	}

	limiter := &RedisAttemptLimiter{}
	attemptKey, failureKey, err := limiter.keys("admin", 42)
	if err != nil {
		t.Fatal(err)
	}
	if attemptKey != "admin_go:wallet:redeem:v1:{admin:42}:attempt" || failureKey != "admin_go:wallet:redeem:v1:{admin:42}:failures" {
		t.Fatalf("keys=%q,%q", attemptKey, failureKey)
	}
	for _, script := range []string{attemptReleaseScript, failureRecordScript, failureStateScript} {
		if strings.Contains(script, "fencing") {
			t.Fatalf("script must not use a fencing key: %s", script)
		}
	}
	if !strings.Contains(attemptReleaseScript, "GET") || !strings.Contains(attemptReleaseScript, "DEL") || !strings.Contains(failureRecordScript, "PEXPIRE") || !strings.Contains(failureStateScript, "PTTL") {
		t.Fatal("Lua contract is incomplete")
	}
}

func TestLimiterRejectsInvalidKeyParts(t *testing.T) {
	limiter := &RedisAttemptLimiter{}
	for _, test := range []struct {
		platform string
		userID   int64
	}{{"", 1}, {"admin:bad", 1}, {"admin", 0}} {
		if _, _, err := limiter.keys(test.platform, test.userID); err == nil {
			t.Fatalf("keys(%q, %d) error=nil", test.platform, test.userID)
		}
	}
}

type fakeAttemptLimiter struct {
	acquireFn       func(context.Context, string, int64) (AttemptLease, error)
	failureStateFn  func(context.Context, string, int64) (FailureState, error)
	recordFailureFn func(context.Context, string, int64) (FailureState, error)
	releaseFn       func(context.Context, AttemptLease) error
}

func (fake *fakeAttemptLimiter) Acquire(ctx context.Context, platform string, userID int64) (AttemptLease, error) {
	return fake.acquireFn(ctx, platform, userID)
}

func (fake *fakeAttemptLimiter) FailureState(ctx context.Context, platform string, userID int64) (FailureState, error) {
	return fake.failureStateFn(ctx, platform, userID)
}

func (fake *fakeAttemptLimiter) RecordFailure(ctx context.Context, platform string, userID int64) (FailureState, error) {
	return fake.recordFailureFn(ctx, platform, userID)
}

func (fake *fakeAttemptLimiter) Release(ctx context.Context, lease AttemptLease) error {
	return fake.releaseFn(ctx, lease)
}
