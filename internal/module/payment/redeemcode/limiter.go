package redeemcode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redeemRedisPrefix = "admin_go:wallet:redeem:v1:"
	failureLimit      = 10
	failureWindow     = 10 * time.Minute
	attemptLockTTL    = 15 * time.Second
	attemptTimeout    = 10 * time.Second
	cleanupTimeout    = 2 * time.Second
)

type AttemptLimiter interface {
	Acquire(context.Context, string, int64) (AttemptLease, error)
	FailureState(context.Context, string, int64) (FailureState, error)
	RecordFailure(context.Context, string, int64) (FailureState, error)
	Release(context.Context, AttemptLease) error
}

type AttemptLease struct {
	Key        string
	Owner      string
	Platform   string
	UserID     int64
	RetryAfter int
}

type FailureState struct {
	Count      int
	TTL        time.Duration
	RetryAfter int
}

var (
	ErrAttemptLocked = errors.New("redeem attempt is already in progress")
	keyPartPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	ownerPattern     = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type AttemptLockedError struct{ RetryAfter int }

func (err *AttemptLockedError) Error() string {
	return fmt.Sprintf("%v (retry after %ds)", ErrAttemptLocked, err.RetryAfter)
}
func (err *AttemptLockedError) Unwrap() error { return ErrAttemptLocked }

const attemptReleaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

const failureRecordScript = `
local current = redis.call("GET", KEYS[1])
if current ~= false and not string.match(current, "^%d+$") then
  return redis.error_reply("redeem failure counter is corrupt")
end
redis.call("INCR", KEYS[1])
local ttl = redis.call("PTTL", KEYS[1])
if ttl < 0 or ttl > tonumber(ARGV[1]) then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
  ttl = redis.call("PTTL", KEYS[1])
end
return {redis.call("GET", KEYS[1]), ttl}
`

const failureStateScript = `
local current = redis.call("GET", KEYS[1])
if current == false then
  return {0, 0}
end
if not string.match(current, "^%d+$") then
  return redis.error_reply("redeem failure counter is corrupt")
end
local ttl = redis.call("PTTL", KEYS[1])
if ttl < 0 or ttl > tonumber(ARGV[1]) then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
  ttl = redis.call("PTTL", KEYS[1])
end
return {current, ttl}
`

type redisLimiterClient interface {
	Do(context.Context, ...interface{}) *redis.Cmd
	PTTL(context.Context, string) *redis.DurationCmd
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
}

type RedisAttemptLimiter struct {
	client redisLimiterClient
	random io.Reader
}

func NewRedisAttemptLimiter(client redisLimiterClient) *RedisAttemptLimiter {
	return &RedisAttemptLimiter{client: client, random: rand.Reader}
}

func (limiter *RedisAttemptLimiter) configured() bool {
	return limiter != nil && !isNilLimiterClient(limiter.client)
}

func isNilLimiterClient(client redisLimiterClient) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (limiter *RedisAttemptLimiter) keys(platform string, userID int64) (string, string, error) {
	platform = strings.TrimSpace(platform)
	if !keyPartPattern.MatchString(platform) || userID <= 0 {
		return "", "", errors.New("redeem limiter: invalid key parts")
	}
	tag := fmt.Sprintf("{%s:%d}", platform, userID)
	base := redeemRedisPrefix + tag
	return base + ":attempt", base + ":failures", nil
}

func (limiter *RedisAttemptLimiter) Acquire(ctx context.Context, platform string, userID int64) (AttemptLease, error) {
	if !limiter.configured() {
		return AttemptLease{}, errLimiterUnavailable
	}
	attemptKey, _, err := limiter.keys(platform, userID)
	if err != nil {
		return AttemptLease{}, err
	}
	ownerBytes := make([]byte, 16)
	reader := limiter.random
	if reader == nil {
		reader = rand.Reader
	}
	if _, err := io.ReadFull(reader, ownerBytes); err != nil {
		return AttemptLease{}, fmt.Errorf("redeem limiter: owner: %w", err)
	}
	owner := hex.EncodeToString(ownerBytes)
	result, err := limiter.client.Do(ctx, attemptAcquireArgs(attemptKey, owner)...).Result()
	acquired := err == nil && result != nil && strings.EqualFold(fmt.Sprint(result), "OK")
	if errors.Is(err, redis.Nil) {
		err = nil
	}
	if err != nil {
		return AttemptLease{}, fmt.Errorf("redeem limiter: acquire: %w", err)
	}
	if !acquired {
		pttl, pttlErr := limiter.client.PTTL(ctx, attemptKey).Result()
		if pttlErr != nil {
			return AttemptLease{}, fmt.Errorf("redeem limiter: busy ttl: %w", pttlErr)
		}
		return AttemptLease{}, &AttemptLockedError{RetryAfter: retryAfter(pttl)}
	}
	return AttemptLease{Key: attemptKey, Owner: owner, Platform: strings.TrimSpace(platform), UserID: userID}, nil
}

func attemptAcquireArgs(key, owner string) []interface{} {
	return []interface{}{"SET", key, owner, "NX", "PX", strconv.FormatInt(attemptLockTTL.Milliseconds(), 10)}
}

func (limiter *RedisAttemptLimiter) FailureState(ctx context.Context, platform string, userID int64) (FailureState, error) {
	if !limiter.configured() {
		return FailureState{}, errLimiterUnavailable
	}
	_, failureKey, err := limiter.keys(platform, userID)
	if err != nil {
		return FailureState{}, err
	}
	return limiter.evalFailure(ctx, failureStateScript, failureKey)
}

func (limiter *RedisAttemptLimiter) RecordFailure(ctx context.Context, platform string, userID int64) (FailureState, error) {
	if !limiter.configured() {
		return FailureState{}, errLimiterUnavailable
	}
	_, failureKey, err := limiter.keys(platform, userID)
	if err != nil {
		return FailureState{}, err
	}
	return limiter.evalFailure(ctx, failureRecordScript, failureKey)
}

func (limiter *RedisAttemptLimiter) evalFailure(ctx context.Context, script, key string) (FailureState, error) {
	values, err := limiter.client.Eval(ctx, script, []string{key}, failureWindow.Milliseconds()).Slice()
	if err != nil {
		return FailureState{}, fmt.Errorf("redeem limiter: failure state: %w", err)
	}
	if len(values) != 2 {
		return FailureState{}, errors.New("redeem limiter: invalid failure state")
	}
	count, ok := redisInt64(values[0])
	ttlMillis, ttlOK := redisInt64(values[1])
	maxInt := int64(^uint(0) >> 1)
	maxTTL := int64(failureWindow / time.Millisecond)
	if !ok || !ttlOK || count < 0 || count > maxInt || ttlMillis < 0 || ttlMillis > maxTTL {
		return FailureState{}, errors.New("redeem limiter: invalid failure state values")
	}
	state := FailureState{Count: int(count), TTL: time.Duration(ttlMillis) * time.Millisecond}
	state.RetryAfter = retryAfter(state.TTL)
	return state, nil
}

func (limiter *RedisAttemptLimiter) Release(ctx context.Context, lease AttemptLease) error {
	if !limiter.configured() {
		return errLimiterUnavailable
	}
	if err := validateAttemptLease(lease); err != nil {
		return err
	}
	result, err := limiter.client.Eval(ctx, attemptReleaseScript, []string{lease.Key}, lease.Owner).Int64()
	if err != nil {
		return fmt.Errorf("redeem limiter: release: %w", err)
	}
	if result != 1 {
		return errors.New("redeem limiter: lease lost")
	}
	return nil
}

func validateAttemptLease(lease AttemptLease) error {
	attemptKey, _, keyErr := (&RedisAttemptLimiter{}).keys(lease.Platform, lease.UserID)
	if keyErr != nil || lease.Key != attemptKey || !ownerPattern.MatchString(lease.Owner) {
		return errors.New("redeem limiter: invalid lease")
	}
	return nil
}

func retryAfter(ttl time.Duration) int {
	if ttl <= 0 {
		return 1
	}
	seconds := int(math.Ceil(float64(ttl) / float64(time.Second)))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func redisInt64(value interface{}) (int64, bool) {
	switch value := value.(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case string:
		return parseRedisDecimal(value)
	case []byte:
		return parseRedisDecimal(string(value))
	default:
		return 0, false
	}
}

func parseRedisDecimal(value string) (int64, bool) {
	if value == "" {
		return 0, false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

var (
	_ AttemptLimiter     = (*RedisAttemptLimiter)(nil)
	_ redisLimiterClient = (*redis.Client)(nil)
)
