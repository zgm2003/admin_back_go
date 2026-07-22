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
	"admin_back_go/internal/infra/redislock"
	"admin_back_go/internal/module/auth/verifycode"
)

var errVerificationDeliveryStateLost = errors.New("verification delivery state lost")

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

const setPendingDeliveryScript = `
if redis.call("HGET", KEYS[2], "owner") ~= ARGV[1]
  or redis.call("HGET", KEYS[2], "token") ~= ARGV[2] then
  return 0
end
redis.call("DEL", KEYS[1])
redis.call("HSET", KEYS[1],
  "version", "1",
  "state", "pending",
  "owner", ARGV[1],
  "code", ARGV[3])
redis.call("PEXPIRE", KEYS[1], ARGV[4])
return 1
`

const commitDeliveryScript = `
if redis.call("HGET", KEYS[2], "owner") ~= ARGV[1]
  or redis.call("HGET", KEYS[2], "token") ~= ARGV[2] then
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
local cache_ok = 0
local cache_type = redis.call("TYPE", KEYS[1]).ok
if cache_type == "none" then
  cache_ok = 1
elseif cache_type == "hash"
  and redis.call("HGET", KEYS[1], "owner") == ARGV[1]
  and redis.call("HGET", KEYS[1], "state") == "pending" then
  redis.call("DEL", KEYS[1])
  cache_ok = 1
end

local lease_ok = 0
if redis.call("HGET", KEYS[2], "owner") == ARGV[1]
  and redis.call("HGET", KEYS[2], "token") == ARGV[2] then
  redis.call("DEL", KEYS[2])
  lease_ok = 1
end
return cache_ok * 2 + lease_ok
`

type CodeStore interface {
	Set(ctx context.Context, key string, code string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type verifyCodeDeliveryStore interface {
	AcquireDelivery(context.Context, string, time.Duration) (redislock.Lease, error)
	SetPendingDelivery(context.Context, redislock.Lease, string, string, time.Duration) error
	RenewDelivery(context.Context, redislock.Lease, time.Duration) error
	CommitDelivery(context.Context, redislock.Lease, string) error
	RollbackDelivery(context.Context, redislock.Lease, string) error
}

type RedisCodeStore struct {
	client     *redisclient.Client
	leaseStore redislock.LeaseStore
}

func NewRedisCodeStore(client *redisclient.Client) CodeStore {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisCodeStore{client: client, leaseStore: redislock.New(client.Redis)}
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

func (s *RedisCodeStore) AcquireDelivery(ctx context.Context, key string, ttl time.Duration) (redislock.Lease, error) {
	if s == nil || s.leaseStore == nil {
		return redislock.Lease{}, ErrRepositoryNotConfigured
	}
	owner, err := randomVerificationDeliveryOwner()
	if err != nil {
		return redislock.Lease{}, fmt.Errorf("generate verification delivery owner: %w", err)
	}
	return s.leaseStore.Acquire(ctx, key+":delivery-lock", owner, ttl)
}

func (s *RedisCodeStore) SetPendingDelivery(ctx context.Context, lease redislock.Lease, key, code string, ttl time.Duration) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	result, err := s.client.Redis.Eval(
		ctx,
		setPendingDeliveryScript,
		[]string{key, lease.Key},
		lease.Owner,
		lease.Token,
		code,
		ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return fmt.Errorf("set pending verification delivery: %w", err)
	}
	if result != 1 {
		return redislock.ErrLeaseLost
	}
	return nil
}

func (s *RedisCodeStore) RenewDelivery(ctx context.Context, lease redislock.Lease, ttl time.Duration) error {
	if s == nil || s.leaseStore == nil {
		return ErrRepositoryNotConfigured
	}
	_, err := s.leaseStore.Renew(ctx, lease, ttl)
	return err
}

func (s *RedisCodeStore) CommitDelivery(ctx context.Context, lease redislock.Lease, key string) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	result, err := s.client.Redis.Eval(ctx, commitDeliveryScript, []string{key, lease.Key}, lease.Owner, lease.Token).Int64()
	if err != nil {
		return fmt.Errorf("commit verification delivery: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("%w: commit result %d", errVerificationDeliveryStateLost, result)
	}
	return nil
}

func (s *RedisCodeStore) RollbackDelivery(ctx context.Context, lease redislock.Lease, key string) error {
	if s == nil || s.client == nil || s.client.Redis == nil {
		return ErrRepositoryNotConfigured
	}
	result, err := s.client.Redis.Eval(ctx, rollbackDeliveryScript, []string{key, lease.Key}, lease.Owner, lease.Token).Int64()
	if err != nil {
		return fmt.Errorf("rollback verification delivery: %w", err)
	}
	if result != 3 {
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
