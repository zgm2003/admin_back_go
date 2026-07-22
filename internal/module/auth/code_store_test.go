package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/infra/redislock"

	"github.com/redis/go-redis/v9"
)

func TestRandomVerificationDeliveryOwnerFromReader(t *testing.T) {
	owner, err := randomVerificationDeliveryOwnerFromReader(bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatalf("generate owner: %v", err)
	}
	if owner != "00000000000000000000000000000000" {
		t.Fatalf("unexpected owner %q", owner)
	}

	wantErr := errors.New("entropy unavailable")
	if _, err := randomVerificationDeliveryOwnerFromReader(errorReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("expected entropy error, got %v", err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestRedisCodeStoreDeliveryProtocolHidesPendingAndRejectsStaleRollback(t *testing.T) {
	client := verificationCodeStoreRedisClient(t)
	store := NewRedisCodeStore(&redisclient.Client{Redis: client}).(*RedisCodeStore)
	ctx := context.Background()
	key := fmt.Sprintf("test:auth:verify-code:%d", time.Now().UnixNano())
	lockKey := key + ":delivery-lock"
	t.Cleanup(func() {
		_ = client.Del(ctx, key, lockKey, lockKey+":fencing-token").Err()
	})

	first, err := store.AcquireDelivery(ctx, key, time.Second)
	if err != nil {
		t.Fatalf("acquire first delivery: %v", err)
	}
	if err := store.SetPendingDelivery(ctx, first, key, "111111", time.Minute); err != nil {
		t.Fatalf("set first pending delivery: %v", err)
	}
	if code, err := store.Get(ctx, key); err != nil || code != "" {
		t.Fatalf("pending code must be hidden: code=%q err=%v", code, err)
	}
	if err := store.CommitDelivery(ctx, first, key); err != nil {
		t.Fatalf("commit first delivery: %v", err)
	}
	if code, err := store.Get(ctx, key); err != nil || code != "111111" {
		t.Fatalf("delivered code unavailable: code=%q err=%v", code, err)
	}

	second, err := store.AcquireDelivery(ctx, key, time.Second)
	if err != nil {
		t.Fatalf("acquire second delivery: %v", err)
	}
	if err := store.SetPendingDelivery(ctx, second, key, "222222", time.Minute); err != nil {
		t.Fatalf("set second pending delivery: %v", err)
	}
	if err := store.RollbackDelivery(ctx, first, key); err == nil {
		t.Fatal("stale owner unexpectedly rolled back replacement delivery")
	}
	if code, err := store.Get(ctx, key); err != nil || code != "" {
		t.Fatalf("replacement pending code became visible: code=%q err=%v", code, err)
	}
	if err := store.RollbackDelivery(ctx, second, key); err != nil {
		t.Fatalf("rollback second delivery: %v", err)
	}
	if err := store.Set(ctx, key, "333333", time.Minute); err != nil {
		t.Fatalf("set legacy code: %v", err)
	}
	if code, err := store.Get(ctx, key); err != nil || code != "333333" {
		t.Fatalf("legacy string code unavailable: code=%q err=%v", code, err)
	}
}

func verificationCodeStoreRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is required for Redis verification-code integration tests")
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

type fakeDeliveryCodeStoreState struct {
	mu sync.Mutex

	deliveryLease     *redislock.Lease
	deliveryPending   map[string]fakePendingDelivery
	deliverySetErr    error
	acquireErr        error
	renewErr          error
	commitErr         error
	rollbackErr       error
	releaseErr        error
	renewCalled       chan struct{}
	renewCalls        int
	nextDeliveryToken uint64
}

type fakePendingDelivery struct {
	owner string
	code  string
}

func (f *fakeCodeStore) AcquireDelivery(_ context.Context, key string, ttl time.Duration) (redislock.Lease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acquireErr != nil {
		return redislock.Lease{}, f.acquireErr
	}
	if f.deliveryLease != nil {
		return redislock.Lease{}, redislock.ErrNotAcquired
	}
	f.nextDeliveryToken++
	lease := redislock.Lease{
		Key:       key + ":delivery-lock",
		Owner:     fmt.Sprintf("attempt-%d", f.nextDeliveryToken),
		Token:     f.nextDeliveryToken,
		ExpiresAt: time.Now().Add(ttl),
	}
	f.deliveryLease = &lease
	return lease, nil
}

func (f *fakeCodeStore) SetPendingDelivery(_ context.Context, lease redislock.Lease, key, code string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deliverySetErr != nil {
		return f.deliverySetErr
	}
	if f.setErr != nil {
		return f.setErr
	}
	if f.err != nil {
		return f.err
	}
	if f.deliveryLease == nil || f.deliveryLease.Owner != lease.Owner || f.deliveryLease.Token != lease.Token {
		return redislock.ErrLeaseLost
	}
	if f.deliveryPending == nil {
		f.deliveryPending = map[string]fakePendingDelivery{}
	}
	f.deliveryPending[key] = fakePendingDelivery{owner: lease.Owner, code: code}
	f.setKey, f.setCode, f.setTTL = key, code, ttl
	return nil
}

func (f *fakeCodeStore) RenewDelivery(_ context.Context, lease redislock.Lease, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewCalls++
	if f.renewCalled != nil {
		select {
		case f.renewCalled <- struct{}{}:
		default:
		}
	}
	if f.renewErr != nil {
		return f.renewErr
	}
	if f.deliveryLease == nil || f.deliveryLease.Owner != lease.Owner || f.deliveryLease.Token != lease.Token {
		return redislock.ErrLeaseLost
	}
	f.deliveryLease.ExpiresAt = time.Now().Add(ttl)
	return nil
}

func (f *fakeCodeStore) CommitDelivery(ctx context.Context, lease redislock.Lease, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captureDeliveryCleanupContext(ctx)
	if f.commitErr != nil {
		return f.commitErr
	}
	if f.releaseErr != nil {
		return f.releaseErr
	}
	pending, ok := f.deliveryPending[key]
	if f.deliveryLease == nil || f.deliveryLease.Owner != lease.Owner || f.deliveryLease.Token != lease.Token || !ok || pending.owner != lease.Owner {
		return redislock.ErrLeaseLost
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = pending.code
	delete(f.deliveryPending, key)
	f.deliveryLease = nil
	return nil
}

func (f *fakeCodeStore) RollbackDelivery(ctx context.Context, lease redislock.Lease, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captureDeliveryCleanupContext(ctx)
	if f.rollbackErr != nil {
		return f.rollbackErr
	}
	if f.releaseErr != nil {
		return f.releaseErr
	}
	if pending, ok := f.deliveryPending[key]; ok && pending.owner == lease.Owner {
		delete(f.deliveryPending, key)
		f.deleted = key
	}
	if f.deliveryLease != nil && f.deliveryLease.Owner == lease.Owner && f.deliveryLease.Token == lease.Token {
		f.deliveryLease = nil
	}
	return nil
}

func (f *fakeCodeStore) captureDeliveryCleanupContext(ctx context.Context) {
	f.deleteCtx = ctx
	f.deleteCtxErr = ctx.Err()
	_, f.deleteCtxHadDeadline = ctx.Deadline()
}
