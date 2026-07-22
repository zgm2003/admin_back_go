package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/module/auth/verifycode"
)

var (
	errVerificationDeliveryInProgress = errors.New("verification delivery already in progress")
	errVerificationDeliveryStateLost  = errors.New("verification delivery state lost")
)

const getVerificationCodeScript = `
local value_type = redis.call("TYPE", KEYS[1]).ok
if value_type == "string" then
  return redis.call("GET", KEYS[1]) or ""
end
if value_type == "hash" and redis.call("HGET", KEYS[1], "state") == "delivered" then
  return redis.call("HGET", KEYS[1], "code") or ""
end
return ""
`

const acquireDeliveryLeaseScript = `
if redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2]) then
  return 1
end
return 0
`

const setPendingDeliveryScript = `
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
  return 0
end
redis.call("DEL", KEYS[1])
redis.call("HSET", KEYS[1],
  "version", "1",
  "state", "pending",
  "owner", ARGV[1],
  "code", ARGV[2])
redis.call("PEXPIRE", KEYS[1], ARGV[3])
return 1
`

const renewDeliveryLeaseScript = `
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
  return 0
end
return redis.call("PEXPIRE", KEYS[1], ARGV[2])
`

const commitDeliveryScript = `
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
  return -1
end
if redis.call("TYPE", KEYS[1]).ok ~= "hash"
  or redis.call("HGET", KEYS[1], "owner") ~= ARGV[1]
  or redis.call("HGET", KEYS[1], "state") ~= "pending" then
  return -2
end
redis.call("HSET", KEYS[1], "state", "delivered")
redis.call("DEL", KEYS[2])
return 1
`

const rollbackDeliveryScript = `
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
  return -1
end
local cache_type = redis.call("TYPE", KEYS[1]).ok
if cache_type == "hash"
  and redis.call("HGET", KEYS[1], "owner") == ARGV[1]
  and redis.call("HGET", KEYS[1], "state") == "pending" then
  redis.call("DEL", KEYS[1])
elseif cache_type ~= "none" then
  return -2
end
redis.call("DEL", KEYS[2])
return 1
`

type CodeStore interface {
	Set(ctx context.Context, key string, code string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type verifyCodeDeliveryLease struct {
	key   string
	owner string
}

type verifyCodeDeliveryStore interface {
	AcquireDelivery(context.Context, string, time.Duration) (verifyCodeDeliveryLease, error)
	SetPendingDelivery(context.Context, verifyCodeDeliveryLease, string, string, time.Duration) error
	RenewDelivery(context.Context, verifyCodeDeliveryLease, time.Duration) error
	CommitDelivery(context.Context, verifyCodeDeliveryLease, string) error
	RollbackDelivery(context.Context, verifyCodeDeliveryLease, string) error
}

type RedisCodeStore struct {
	client *redisclient.Client
}

func NewRedisCodeStore(client *redisclient.Client) CodeStore {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisCodeStore{client: client}
}

func (s *RedisCodeStore) Set(ctx context.Context, key string, code string, ttl time.Duration) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	return s.client.Redis.Set(ctx, key, code, ttl).Err()
}

func (s *RedisCodeStore) Get(ctx context.Context, key string) (string, error) {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return "", ErrRepositoryNotConfigured
	}
	value, err := s.client.Redis.Eval(ctx, getVerificationCodeScript, []string{key}).Text()
	if err != nil {
		return "", fmt.Errorf("get verification code: %w", err)
	}
	return value, nil
}

func (s *RedisCodeStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	return s.client.Redis.Del(ctx, key).Err()
}

func (s *RedisCodeStore) AcquireDelivery(ctx context.Context, key string, ttl time.Duration) (verifyCodeDeliveryLease, error) {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return verifyCodeDeliveryLease{}, ErrRepositoryNotConfigured
	}
	if key == "" || ttl < time.Millisecond {
		return verifyCodeDeliveryLease{}, errors.New("invalid verification delivery lease")
	}
	owner, err := randomVerificationDeliveryOwner()
	if err != nil {
		return verifyCodeDeliveryLease{}, fmt.Errorf("generate verification delivery owner: %w", err)
	}
	lease := verifyCodeDeliveryLease{key: key + ":delivery-lock", owner: owner}
	acquired, err := s.client.Redis.Eval(ctx, acquireDeliveryLeaseScript, []string{lease.key}, lease.owner, ttl.Milliseconds()).Int64()
	if err != nil {
		return verifyCodeDeliveryLease{}, fmt.Errorf("acquire verification delivery lease: %w", err)
	}
	if acquired != 1 {
		return verifyCodeDeliveryLease{}, errVerificationDeliveryInProgress
	}
	return lease, nil
}

func (s *RedisCodeStore) SetPendingDelivery(ctx context.Context, lease verifyCodeDeliveryLease, key, code string, ttl time.Duration) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	if lease.key == "" || lease.owner == "" || ttl < time.Millisecond {
		return errVerificationDeliveryStateLost
	}
	result, err := s.client.Redis.Eval(
		ctx,
		setPendingDeliveryScript,
		[]string{key, lease.key},
		lease.owner,
		code,
		ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return fmt.Errorf("set pending verification delivery: %w", err)
	}
	if result != 1 {
		return errVerificationDeliveryStateLost
	}
	return nil
}

func (s *RedisCodeStore) RenewDelivery(ctx context.Context, lease verifyCodeDeliveryLease, ttl time.Duration) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	if lease.key == "" || lease.owner == "" || ttl < time.Millisecond {
		return errVerificationDeliveryStateLost
	}
	result, err := s.client.Redis.Eval(ctx, renewDeliveryLeaseScript, []string{lease.key}, lease.owner, ttl.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("renew verification delivery lease: %w", err)
	}
	if result != 1 {
		return errVerificationDeliveryStateLost
	}
	return nil
}

func (s *RedisCodeStore) CommitDelivery(ctx context.Context, lease verifyCodeDeliveryLease, key string) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	if lease.key == "" || lease.owner == "" {
		return errVerificationDeliveryStateLost
	}
	result, err := s.client.Redis.Eval(ctx, commitDeliveryScript, []string{key, lease.key}, lease.owner).Int64()
	if err != nil {
		return fmt.Errorf("commit verification delivery: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("%w: commit result %d", errVerificationDeliveryStateLost, result)
	}
	return nil
}

func (s *RedisCodeStore) RollbackDelivery(ctx context.Context, lease verifyCodeDeliveryLease, key string) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	if lease.key == "" || lease.owner == "" {
		return errVerificationDeliveryStateLost
	}
	result, err := s.client.Redis.Eval(ctx, rollbackDeliveryScript, []string{key, lease.key}, lease.owner).Int64()
	if err != nil {
		return fmt.Errorf("rollback verification delivery: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("%w: rollback result %d", errVerificationDeliveryStateLost, result)
	}
	return nil
}

func randomVerificationDeliveryOwner() (string, error) {
	return randomVerificationDeliveryOwnerFromReader(rand.Reader)
}

func randomVerificationDeliveryOwnerFromReader(reader io.Reader) (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func VerifyCodeCacheKey(accountType string, scene string, account string) string {
	return verifycode.CacheKey(accountType, scene, account)
}

var _ verifyCodeDeliveryStore = (*RedisCodeStore)(nil)
