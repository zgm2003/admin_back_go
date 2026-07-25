package redeemcode

import (
	"context"
	"fmt"
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
	command := attemptAcquireArgs("attempt-key", "0123456789abcdef0123456789abcdef")
	if fmt.Sprint(command) != "[SET attempt-key 0123456789abcdef0123456789abcdef NX PX 15000]" {
		t.Fatalf("acquire command=%v", command)
	}
	if !strings.Contains(failureRecordScript, "KEYS[1]") || !strings.Contains(failureStateScript, "KEYS[1]") || strings.Contains(failureRecordScript+failureStateScript, "KEYS[2]") {
		t.Fatal("failure scripts must be single-key atomic operations")
	}
	if !strings.Contains(failureRecordScript, "local ttl = redis.call(\"PTTL\"") || !strings.Contains(failureRecordScript, "if ttl < 0 then\n  redis.call(\"PEXPIRE\"") {
		t.Fatal("failure record must preserve an existing TTL and only restore an anomalous one")
	}
	if !strings.Contains(failureStateScript, "if ttl < 0 then\n  redis.call(\"PEXPIRE\"") || !strings.Contains(failureStateScript, "ARGV[1]") {
		t.Fatal("failure state must restore a missing TTL to its fixed window")
	}
	if failureLimit != 10 || failureWindow.Milliseconds() != 600000 || attemptLockTTL.Milliseconds() != 15000 {
		t.Fatalf("literal limiter contract changed: limit=%d failureMs=%d lockMs=%d", failureLimit, failureWindow.Milliseconds(), attemptLockTTL.Milliseconds())
	}
}

func TestLimiterReleaseRejectsForgedNamespaceOrOwner(t *testing.T) {
	limiter := NewRedisAttemptLimiter(nil)
	for _, lease := range []AttemptLease{
		{Key: "other:key", Platform: "admin", UserID: 42, Owner: "0123456789abcdef0123456789abcdef"},
		{Key: "admin_go:wallet:redeem:v1:{admin:42}:attempt", Platform: "admin", UserID: 42, Owner: "forged"},
		{Key: "admin_go:wallet:redeem:v1:{admin:42}:attempt", Platform: "admin", UserID: 0, Owner: "0123456789abcdef0123456789abcdef"},
	} {
		if err := limiter.Release(context.Background(), lease); err == nil {
			t.Fatalf("forged lease accepted: %+v", lease)
		}
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

func newAllowAttemptLimiter() *fakeAttemptLimiter {
	return &fakeAttemptLimiter{
		acquireFn: func(context.Context, string, int64) (AttemptLease, error) {
			return AttemptLease{Key: "admin_go:wallet:redeem:v1:{admin:7}:attempt", Owner: "0123456789abcdef0123456789abcdef", Platform: "admin", UserID: 7}, nil
		},
		failureStateFn: func(context.Context, string, int64) (FailureState, error) { return FailureState{}, nil },
		recordFailureFn: func(context.Context, string, int64) (FailureState, error) {
			return FailureState{Count: 1, TTL: failureWindow}, nil
		},
		releaseFn: func(context.Context, AttemptLease) error { return nil },
	}
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
