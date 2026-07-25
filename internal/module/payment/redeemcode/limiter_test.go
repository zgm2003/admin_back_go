package redeemcode

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
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
	if !strings.Contains(failureRecordScript, "local ttl = redis.call(\"PTTL\"") || !strings.Contains(failureRecordScript, "if ttl < 0 or ttl > tonumber(ARGV[1]) then") {
		t.Fatal("failure record must preserve an existing TTL and only restore an anomalous one")
	}
	if !strings.Contains(failureStateScript, "if ttl < 0 or ttl > tonumber(ARGV[1]) then") || !strings.Contains(failureStateScript, "ARGV[1]") {
		t.Fatal("failure state must restore a missing TTL to its fixed window")
	}
	for _, script := range []string{failureRecordScript, failureStateScript} {
		if strings.Contains(script, "tonumber(current)") {
			t.Fatal("failure scripts must return the exact Redis counter string, not a lossy Lua number")
		}
	}
	validationIndex := strings.Index(failureRecordScript, "local current = redis.call(\"GET\"")
	incrementIndex := strings.Index(failureRecordScript, "redis.call(\"INCR\"")
	if validationIndex < 0 || incrementIndex < 0 || validationIndex > incrementIndex {
		t.Fatal("failure record must validate an existing counter before INCR")
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

func TestLimiterTypedNilClientFailsClosedWithoutPanic(t *testing.T) {
	var client *redis.Client
	limiter := NewRedisAttemptLimiter(client)
	lease := AttemptLease{Key: "admin_go:wallet:redeem:v1:{admin:7}:attempt", Owner: "0123456789abcdef0123456789abcdef", Platform: "admin", UserID: 7}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "acquire", call: func() error { _, err := limiter.Acquire(context.Background(), "admin", 7); return err }},
		{name: "failure state", call: func() error { _, err := limiter.FailureState(context.Background(), "admin", 7); return err }},
		{name: "record failure", call: func() error { _, err := limiter.RecordFailure(context.Background(), "admin", 7); return err }},
		{name: "release", call: func() error { return limiter.Release(context.Background(), lease) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("panic=%v", recovered)
				}
			}()
			if err := test.call(); err == nil {
				t.Fatal("error=nil")
			}
		})
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

type scriptedRedisClient struct {
	doValue     interface{}
	doErr       error
	doArgs      []interface{}
	pttlValue   time.Duration
	pttlErr     error
	pttlKey     string
	evalValue   interface{}
	evalErr     error
	evalKey     []string
	evalArgs    []interface{}
	evalScripts []string
	evalCount   int
}

func (fake *scriptedRedisClient) Do(ctx context.Context, args ...interface{}) *redis.Cmd {
	fake.doArgs = append([]interface{}(nil), args...)
	cmd := redis.NewCmd(ctx, args...)
	if fake.doErr != nil {
		cmd.SetErr(fake.doErr)
	} else {
		cmd.SetVal(fake.doValue)
	}
	return cmd
}

func (fake *scriptedRedisClient) PTTL(ctx context.Context, key string) *redis.DurationCmd {
	fake.pttlKey = key
	cmd := redis.NewDurationCmd(ctx, time.Millisecond)
	if fake.pttlErr != nil {
		cmd.SetErr(fake.pttlErr)
	} else {
		cmd.SetVal(fake.pttlValue)
	}
	return cmd
}

func (fake *scriptedRedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	fake.evalCount++
	fake.evalScripts = append(fake.evalScripts, script)
	fake.evalKey = append([]string(nil), keys...)
	fake.evalArgs = append([]interface{}(nil), args...)
	cmd := redis.NewCmd(ctx)
	if fake.evalErr != nil {
		cmd.SetErr(fake.evalErr)
	} else {
		cmd.SetVal(fake.evalValue)
	}
	return cmd
}

func TestRedisAttemptLimiterCommandFake(t *testing.T) {
	ctx := context.Background()
	fake := &scriptedRedisClient{doValue: "OK"}
	limiter := NewRedisAttemptLimiter(fake)
	limiter.random = strings.NewReader(strings.Repeat("x", 16*5))
	lease, err := limiter.Acquire(ctx, "admin", 7)
	if err != nil || fmt.Sprint(fake.doArgs) != "[SET admin_go:wallet:redeem:v1:{admin:7}:attempt 78787878787878787878787878787878 NX PX 15000]" || lease.Owner != strings.Repeat("78", 16) {
		t.Fatalf("lease=%+v err=%v args=%v", lease, err, fake.doArgs)
	}

	fake.doValue = nil
	fake.doErr = redis.Nil
	fake.pttlValue = 1500 * time.Millisecond
	_, err = limiter.Acquire(ctx, "admin", 7)
	var locked *AttemptLockedError
	if !errors.As(err, &locked) || locked.RetryAfter != 2 {
		t.Fatalf("redis.Nil locked error=%v", err)
	}
	if fake.pttlKey != "admin_go:wallet:redeem:v1:{admin:7}:attempt" {
		t.Fatalf("PTTL key=%q", fake.pttlKey)
	}
	fake.pttlValue = -time.Millisecond
	_, err = limiter.Acquire(ctx, "admin", 7)
	if !errors.As(err, &locked) || locked.RetryAfter != 1 {
		t.Fatalf("minimum retry error=%v", err)
	}
	fake.doErr = errors.New("set failed")
	if _, err = limiter.Acquire(ctx, "admin", 7); err == nil {
		t.Fatal("SET error was ignored")
	}
	fake.doErr = redis.Nil
	fake.pttlErr = errors.New("pttl failed")
	if _, err = limiter.Acquire(ctx, "admin", 7); err == nil {
		t.Fatal("PTTL error was ignored")
	}

	fake.evalValue = []interface{}{int64(3), int64(1000)}
	fake.pttlErr = nil
	state, err := limiter.FailureState(ctx, "admin", 7)
	if err != nil || state.Count != 3 || state.TTL != time.Second || len(fake.evalKey) != 1 || fake.evalKey[0] != "admin_go:wallet:redeem:v1:{admin:7}:failures" {
		t.Fatalf("state=%+v err=%v key=%v", state, err, fake.evalKey)
	}
	if _, err = limiter.RecordFailure(ctx, "admin", 7); err != nil || fake.evalCount != 2 {
		t.Fatalf("record err=%v evals=%d", err, fake.evalCount)
	}
	if len(fake.evalScripts) != 2 || fake.evalScripts[0] != failureStateScript || fake.evalScripts[1] != failureRecordScript {
		t.Fatalf("failure scripts=%v", fake.evalScripts)
	}
	if fmt.Sprint(fake.evalArgs) != "[600000]" {
		t.Fatalf("failure script args=%v", fake.evalArgs)
	}
	fake.evalErr = errors.New("failure script failed")
	if _, err = limiter.FailureState(ctx, "admin", 7); err == nil {
		t.Fatal("FailureState Eval error was ignored")
	}
	if _, err = limiter.RecordFailure(ctx, "admin", 7); err == nil {
		t.Fatal("RecordFailure Eval error was ignored")
	}
	fake.evalErr = nil

	lease = AttemptLease{Key: "admin_go:wallet:redeem:v1:{admin:7}:attempt", Platform: "admin", UserID: 7, Owner: strings.Repeat("78", 16)}
	fake.evalValue = int64(1)
	if err = limiter.Release(ctx, lease); err != nil {
		t.Fatalf("release result=1 err=%v", err)
	}
	if len(fake.evalKey) != 1 || fake.evalKey[0] != lease.Key {
		t.Fatalf("release keys=%v", fake.evalKey)
	}
	fake.evalValue = int64(0)
	if err = limiter.Release(ctx, lease); err == nil {
		t.Fatal("release result=0 accepted")
	}
	fake.evalErr = errors.New("release failed")
	if err = limiter.Release(ctx, lease); err == nil {
		t.Fatal("release redis error ignored")
	}
}

func TestRedisAttemptLimiterRejectsCorruptFailureResults(t *testing.T) {
	ctx := context.Background()
	for _, value := range []interface{}{
		[]interface{}{int64(-1), int64(1)},
		[]interface{}{"not-an-int", int64(1)},
		[]interface{}{"1 trailing", int64(1)},
		[]interface{}{[]byte("999999999999999999999999"), int64(1)},
		[]interface{}{uint64(math.MaxUint64), int64(1)},
		[]interface{}{int64(1)},
		[]interface{}{int64(1), int64(-1)},
		[]interface{}{int64(1), int64(600001)},
		[]interface{}{int64(1), int64(math.MaxInt64)},
	} {
		fake := &scriptedRedisClient{evalValue: value}
		limiter := NewRedisAttemptLimiter(fake)
		if _, err := limiter.FailureState(ctx, "admin", 7); err == nil {
			t.Fatalf("corrupt eval value accepted: %#v", value)
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
