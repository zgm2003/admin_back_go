package redislock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrNotAcquired = errors.New("redislock: lease not acquired")
	ErrLeaseLost   = errors.New("redislock: lease lost")
)

const acquireScript = `
if redis.call("EXISTS", KEYS[1]) == 1 then
  return 0
end
local token = redis.call("INCR", KEYS[2])
redis.call("HSET", KEYS[1], "owner", ARGV[1], "token", tostring(token))
redis.call("PEXPIRE", KEYS[1], ARGV[2])
return token
`

const renewScript = `
if redis.call("HGET", KEYS[1], "owner") == ARGV[1]
  and redis.call("HGET", KEYS[1], "token") == ARGV[2] then
  redis.call("PEXPIRE", KEYS[1], ARGV[3])
  return 1
end
return 0
`

const releaseScript = `
if redis.call("HGET", KEYS[1], "owner") == ARGV[1]
  and redis.call("HGET", KEYS[1], "token") == ARGV[2] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

type Lease struct {
	Key       string
	Owner     string
	Token     uint64
	ExpiresAt time.Time
}

type LeaseStore interface {
	Acquire(context.Context, string, string, time.Duration) (Lease, error)
	Renew(context.Context, Lease, time.Duration) (Lease, error)
	Release(context.Context, Lease) error
}

type RedisLeaseStore struct {
	client redis.Cmdable
}

func New(client redis.Cmdable) *RedisLeaseStore {
	return &RedisLeaseStore{client: client}
}

func (s *RedisLeaseStore) Acquire(ctx context.Context, key string, owner string, ttl time.Duration) (Lease, error) {
	key, owner, err := validateAcquireInput(s, key, owner, ttl)
	if err != nil {
		return Lease{}, err
	}
	token, err := s.client.Eval(ctx, acquireScript, []string{key, fencingKey(key)}, owner, ttl.Milliseconds()).Int64()
	if err != nil {
		return Lease{}, fmt.Errorf("redislock: acquire: %w", err)
	}
	if token == 0 {
		return Lease{}, ErrNotAcquired
	}
	if token < 0 {
		return Lease{}, errors.New("redislock: invalid fencing token")
	}
	return Lease{Key: key, Owner: owner, Token: uint64(token), ExpiresAt: time.Now().Add(ttl)}, nil
}

func (s *RedisLeaseStore) Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if err := validateLeaseInput(s, lease); err != nil {
		return Lease{}, err
	}
	if ttl < time.Millisecond {
		return Lease{}, errors.New("redislock: invalid lease ttl")
	}
	result, err := s.client.Eval(ctx, renewScript, []string{lease.Key}, lease.Owner, lease.Token, ttl.Milliseconds()).Int64()
	if err != nil {
		return Lease{}, fmt.Errorf("redislock: renew: %w", err)
	}
	if result != 1 {
		return Lease{}, ErrLeaseLost
	}
	lease.ExpiresAt = time.Now().Add(ttl)
	return lease, nil
}

func (s *RedisLeaseStore) Release(ctx context.Context, lease Lease) error {
	if err := validateLeaseInput(s, lease); err != nil {
		return err
	}
	result, err := s.client.Eval(ctx, releaseScript, []string{lease.Key}, lease.Owner, lease.Token).Int64()
	if err != nil {
		return fmt.Errorf("redislock: release: %w", err)
	}
	if result != 1 {
		return ErrLeaseLost
	}
	return nil
}

func validateAcquireInput(store *RedisLeaseStore, key string, owner string, ttl time.Duration) (string, string, error) {
	if store == nil || store.client == nil {
		return "", "", errors.New("redislock: client not configured")
	}
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if key == "" || owner == "" || ttl < time.Millisecond {
		return "", "", errors.New("redislock: invalid lease input")
	}
	return key, owner, nil
}

func validateLeaseInput(store *RedisLeaseStore, lease Lease) error {
	if store == nil || store.client == nil {
		return errors.New("redislock: client not configured")
	}
	if strings.TrimSpace(lease.Key) == "" || strings.TrimSpace(lease.Owner) == "" || lease.Token == 0 {
		return errors.New("redislock: invalid lease input")
	}
	return nil
}

func fencingKey(key string) string {
	return key + ":fencing-token"
}

var _ LeaseStore = (*RedisLeaseStore)(nil)
