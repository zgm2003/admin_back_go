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
	t.Cleanup(func() {
		_ = client.Del(ctx, key).Err()
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

func TestRedisCodeStoreDeliveryLeaseLifecycleLeavesNoAuxiliaryKeys(t *testing.T) {
	client := verificationCodeStoreRedisClient(t)
	store := NewRedisCodeStore(&redisclient.Client{Redis: client}).(*RedisCodeStore)
	ctx := context.Background()

	t.Run("commit", func(t *testing.T) {
		key := fmt.Sprintf("test:auth:verify-code:commit:%d", time.Now().UnixNano())
		t.Cleanup(func() { _ = client.Del(ctx, key).Err() })
		lease, err := store.AcquireDelivery(ctx, key, time.Second)
		if err != nil {
			t.Fatalf("acquire delivery: %v", err)
		}
		if err := store.SetPendingDelivery(ctx, lease, key, "111111", time.Minute); err != nil {
			t.Fatalf("set pending delivery: %v", err)
		}
		if err := store.CommitDelivery(ctx, lease, key); err != nil {
			t.Fatalf("commit delivery: %v", err)
		}
		assertNoVerificationDeliveryAuxiliaryKeys(t, client, key)
	})

	t.Run("rollback", func(t *testing.T) {
		key := fmt.Sprintf("test:auth:verify-code:rollback:%d", time.Now().UnixNano())
		lease, err := store.AcquireDelivery(ctx, key, time.Second)
		if err != nil {
			t.Fatalf("acquire delivery: %v", err)
		}
		if err := store.SetPendingDelivery(ctx, lease, key, "222222", time.Minute); err != nil {
			t.Fatalf("set pending delivery: %v", err)
		}
		if err := store.RollbackDelivery(ctx, lease, key); err != nil {
			t.Fatalf("rollback delivery: %v", err)
		}
		assertNoVerificationDeliveryAuxiliaryKeys(t, client, key)
	})

	t.Run("lease expiry", func(t *testing.T) {
		key := fmt.Sprintf("test:auth:verify-code:expiry:%d", time.Now().UnixNano())
		if _, err := store.AcquireDelivery(ctx, key, 30*time.Millisecond); err != nil {
			t.Fatalf("acquire delivery: %v", err)
		}
		lockKey := key + ":delivery-lock"
		deadline := time.Now().Add(time.Second)
		for {
			exists, err := client.Exists(ctx, lockKey).Result()
			if err != nil {
				t.Fatalf("inspect delivery lock: %v", err)
			}
			if exists == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("delivery lock did not expire: %q", lockKey)
			}
			time.Sleep(10 * time.Millisecond)
		}
		assertNoVerificationDeliveryAuxiliaryKeys(t, client, key)
	})

	t.Run("unique key batch", func(t *testing.T) {
		prefix := fmt.Sprintf("test:auth:verify-code:batch:%d", time.Now().UnixNano())
		before, err := client.Keys(ctx, prefix+"*").Result()
		if err != nil {
			t.Fatalf("count baseline keys: %v", err)
		}
		for i := 0; i < 16; i++ {
			key := fmt.Sprintf("%s:%02d", prefix, i)
			lease, err := store.AcquireDelivery(ctx, key, time.Second)
			if err != nil {
				t.Fatalf("acquire delivery %d: %v", i, err)
			}
			if err := store.SetPendingDelivery(ctx, lease, key, "333333", time.Minute); err != nil {
				t.Fatalf("set pending delivery %d: %v", i, err)
			}
			if err := store.RollbackDelivery(ctx, lease, key); err != nil {
				t.Fatalf("rollback delivery %d: %v", i, err)
			}
		}
		after, err := client.Keys(ctx, prefix+"*").Result()
		if err != nil {
			t.Fatalf("count keys after delivery lifecycle: %v", err)
		}
		if len(after) != len(before) {
			t.Fatalf("verification delivery lifecycle leaked keys: before=%v after=%v", before, after)
		}
	})
}

func assertNoVerificationDeliveryAuxiliaryKeys(t *testing.T, client *redis.Client, key string) {
	t.Helper()
	keys, err := client.Keys(context.Background(), key+":delivery-lock*").Result()
	if err != nil {
		t.Fatalf("inspect verification delivery auxiliary keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("verification delivery lifecycle leaked auxiliary keys: %v", keys)
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

	deliveryLease     *verifyCodeDeliveryLease
	deliveryPending   map[string]fakePendingDelivery
	deliverySetErr    error
	acquireErr        error
	renewErr          error
	commitErr         error
	rollbackErr       error
	releaseErr        error
	renewCalled       chan struct{}
	renew             func(context.Context, verifyCodeDeliveryLease, time.Duration) error
	renewCalls        int
	nextDeliveryToken uint64
}

type fakePendingDelivery struct {
	owner string
	code  string
}

func (f *fakeCodeStore) AcquireDelivery(_ context.Context, key string, _ time.Duration) (verifyCodeDeliveryLease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acquireErr != nil {
		return verifyCodeDeliveryLease{}, f.acquireErr
	}
	if f.deliveryLease != nil {
		return verifyCodeDeliveryLease{}, errVerificationDeliveryInProgress
	}
	f.nextDeliveryToken++
	lease := verifyCodeDeliveryLease{
		key:   key + ":delivery-lock",
		owner: fmt.Sprintf("attempt-%d", f.nextDeliveryToken),
	}
	f.deliveryLease = &lease
	return lease, nil
}

func (f *fakeCodeStore) SetPendingDelivery(_ context.Context, lease verifyCodeDeliveryLease, key, code string, ttl time.Duration) error {
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
	if f.deliveryLease == nil || f.deliveryLease.key != lease.key || f.deliveryLease.owner != lease.owner {
		return errVerificationDeliveryStateLost
	}
	if f.deliveryPending == nil {
		f.deliveryPending = map[string]fakePendingDelivery{}
	}
	f.deliveryPending[key] = fakePendingDelivery{owner: lease.owner, code: code}
	f.setKey, f.setCode, f.setTTL = key, code, ttl
	return nil
}

func (f *fakeCodeStore) RenewDelivery(ctx context.Context, lease verifyCodeDeliveryLease, ttl time.Duration) error {
	if f.renew != nil {
		return f.renew(ctx, lease, ttl)
	}
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
	if f.deliveryLease == nil || f.deliveryLease.key != lease.key || f.deliveryLease.owner != lease.owner {
		return errVerificationDeliveryStateLost
	}
	return nil
}

func (f *fakeCodeStore) CommitDelivery(ctx context.Context, lease verifyCodeDeliveryLease, key string) error {
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
	if f.deliveryLease == nil || f.deliveryLease.key != lease.key || f.deliveryLease.owner != lease.owner || !ok || pending.owner != lease.owner {
		return errVerificationDeliveryStateLost
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = pending.code
	delete(f.deliveryPending, key)
	f.deliveryLease = nil
	return nil
}

func (f *fakeCodeStore) RollbackDelivery(ctx context.Context, lease verifyCodeDeliveryLease, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captureDeliveryCleanupContext(ctx)
	if f.rollbackErr != nil {
		return f.rollbackErr
	}
	if f.releaseErr != nil {
		return f.releaseErr
	}
	if pending, ok := f.deliveryPending[key]; ok && pending.owner == lease.owner {
		delete(f.deliveryPending, key)
		f.deleted = key
	}
	if f.deliveryLease != nil && f.deliveryLease.key == lease.key && f.deliveryLease.owner == lease.owner {
		f.deliveryLease = nil
	}
	return nil
}

func (f *fakeCodeStore) captureDeliveryCleanupContext(ctx context.Context) {
	f.deleteCtx = ctx
	f.deleteCtxErr = ctx.Err()
	_, f.deleteCtxHadDeadline = ctx.Deadline()
}
