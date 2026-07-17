package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/redislock"
	"admin_back_go/internal/telemetry"

	"github.com/redis/go-redis/v9"
)

func TestNewRejectsInvalidTimezone(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "bad/timezone"})
	if err == nil {
		t.Fatal("expected invalid timezone to be rejected")
	}
	if scheduler != nil || !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("expected ErrInvalidTimezone and nil scheduler, got scheduler=%#v err=%v", scheduler, err)
	}
}

func TestNewUsesConfiguredTimezone(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })
	if scheduler.location.String() != "Asia/Shanghai" {
		t.Fatalf("expected Asia/Shanghai location, got %s", scheduler.location)
	}
}

func TestNewUsesCodeOwnedLeaseDefaultsForZeroConfig(t *testing.T) {
	store := newFakeLeaseStore()
	scheduler, err := New(config.SchedulerConfig{}, WithLeaseStore(store))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	run := false
	if err := scheduler.wrapTask("job-a", func(context.Context) error {
		run = true
		return nil
	})(context.Background()); err != nil {
		t.Fatalf("task returned error: %v", err)
	}
	if !run {
		t.Fatal("expected task to run")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.acquireKey != config.DefaultSchedulerLockPrefix+"job-a" || store.acquireTTL != config.DefaultSchedulerLockTTL {
		t.Fatalf("unexpected acquire call: key=%q ttl=%s", store.acquireKey, store.acquireTTL)
	}
}

func TestEveryRejectsInvalidDefinition(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	if err := scheduler.Every("", time.Minute, func(context.Context) error { return nil }); !errors.Is(err, ErrJobNameRequired) {
		t.Fatalf("expected ErrJobNameRequired, got %v", err)
	}
	if err := scheduler.Every("job", 0, func(context.Context) error { return nil }); !errors.Is(err, ErrJobIntervalRequired) {
		t.Fatalf("expected ErrJobIntervalRequired, got %v", err)
	}
	if err := scheduler.Every("job", time.Minute, nil); !errors.Is(err, ErrJobTaskRequired) {
		t.Fatalf("expected ErrJobTaskRequired, got %v", err)
	}
}

func TestSchedulerRenewsLeasePastThreeTTLs(t *testing.T) {
	store := newFakeLeaseStore()
	store.renewed = make(chan struct{}, 8)
	scheduler, err := New(
		config.SchedulerConfig{Timezone: "UTC", LockPrefix: "test:scheduler:", LockTTL: 60 * time.Millisecond},
		WithLeaseStore(store),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	err = scheduler.wrapTask("long-job", func(ctx context.Context) error {
		for range 3 {
			select {
			case <-store.renewed:
			case <-time.After(time.Second):
				return errors.New("lease was not renewed")
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})(context.Background())
	if err != nil {
		t.Fatalf("long task returned error: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.renewCalls < 3 || store.releaseCalls != 1 {
		t.Fatalf("expected at least three renewals and one release, renew=%d release=%d", store.renewCalls, store.releaseCalls)
	}
}

func TestSchedulerCancelsCallbackWhenLeaseRenewalIsLost(t *testing.T) {
	store := newFakeLeaseStore()
	store.renewErr = redislock.ErrLeaseLost
	scheduler, err := New(
		config.SchedulerConfig{Timezone: "UTC", LockPrefix: "test:scheduler:", LockTTL: 30 * time.Millisecond},
		WithLeaseStore(store),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	enqueued := false
	err = scheduler.wrapTask("lease-loss", func(ctx context.Context) error {
		<-ctx.Done()
		if ctx.Err() == nil {
			enqueued = true
		}
		return ctx.Err()
	})(context.Background())
	if !errors.Is(err, redislock.ErrLeaseLost) {
		t.Fatalf("expected lease loss error, got %v", err)
	}
	if enqueued {
		t.Fatal("callback continued with an active context after lease loss")
	}
}

func TestSchedulerDoesNotReportLeaseLossWhenCompletedTaskStopsInFlightRenewal(t *testing.T) {
	store := newFakeLeaseStore()
	store.renewStarted = make(chan struct{})
	store.blockRenewUntilCanceled = true
	scheduler, err := New(
		config.SchedulerConfig{Timezone: "UTC", LockPrefix: "test:scheduler:", LockTTL: 30 * time.Millisecond},
		WithLeaseStore(store),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	err = scheduler.wrapTask("quick-finish", func(ctx context.Context) error {
		select {
		case <-store.renewStarted:
			return nil
		case <-time.After(time.Second):
			return errors.New("renewal did not start")
		case <-ctx.Done():
			return ctx.Err()
		}
	})(context.Background())
	if err != nil {
		t.Fatalf("normal completion raced with renewal shutdown: %v", err)
	}
}

func TestSchedulerSkipsWhenLeaseNotAcquired(t *testing.T) {
	store := newFakeLeaseStore()
	store.acquireErr = redislock.ErrNotAcquired
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC"}, WithLeaseStore(store))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	run := false
	if err := scheduler.wrapTask("job-a", func(context.Context) error { run = true; return nil })(context.Background()); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
	if run {
		t.Fatal("task must not run when another owner holds the lease")
	}
}

func TestSchedulerCronAddUpdateRemoveAndList(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })
	task := func(context.Context) error { return nil }

	if err := scheduler.AddCron("dynamic-job", "*/5 * * * * *", true, task); err != nil {
		t.Fatalf("AddCron returned error: %v", err)
	}
	if err := scheduler.AddCron("dynamic-job", "*/5 * * * * *", true, task); !errors.Is(err, ErrJobAlreadyExists) {
		t.Fatalf("duplicate AddCron must fail with ErrJobAlreadyExists, got %v", err)
	}
	jobs := scheduler.List()
	if len(jobs) != 1 || jobs[0].Name != "dynamic-job" || jobs[0].Expression != "*/5 * * * * *" {
		t.Fatalf("unexpected jobs after add: %#v", jobs)
	}

	if err := scheduler.UpdateCron("dynamic-job", "*/10 * * * * *", true, task); err != nil {
		t.Fatalf("UpdateCron returned error: %v", err)
	}
	jobs = scheduler.List()
	if len(jobs) != 1 || jobs[0].Expression != "*/10 * * * * *" {
		t.Fatalf("unexpected jobs after update: %#v", jobs)
	}
	if err := scheduler.UpdateCron("missing", "*/10 * * * * *", true, task); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing update must fail with ErrJobNotFound, got %v", err)
	}

	if err := scheduler.Remove("dynamic-job"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if jobs := scheduler.List(); len(jobs) != 0 {
		t.Fatalf("expected no jobs after remove, got %#v", jobs)
	}
	if err := scheduler.Remove("dynamic-job"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("second remove must fail with ErrJobNotFound, got %v", err)
	}
}

func TestMultiWorkerSchedulersShareRedisLeaseWhileLongCallbackRenews(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is required for multi-scheduler Redis test")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect test Redis: %v", err)
	}
	prefix := fmt.Sprintf("test:scheduler:multi:%d:", time.Now().UnixNano())
	key := prefix + "shared-job"
	t.Cleanup(func() { _ = client.Del(context.Background(), key, key+":fencing-token").Err() })
	cfg := config.SchedulerConfig{Timezone: "UTC", LockPrefix: prefix, LockTTL: 120 * time.Millisecond}
	first, err := New(cfg, WithLeaseStore(redislock.New(client)))
	if err != nil {
		t.Fatalf("New first scheduler: %v", err)
	}
	second, err := New(cfg, WithLeaseStore(redislock.New(client)))
	if err != nil {
		t.Fatalf("New second scheduler: %v", err)
	}
	t.Cleanup(func() {
		_ = first.Shutdown(context.Background())
		_ = second.Shutdown(context.Background())
	})

	started := make(chan struct{})
	var executions atomic.Int32
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- first.wrapTask("shared-job", func(ctx context.Context) error {
			executions.Add(1)
			close(started)
			select {
			case <-time.After(420 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})(ctx)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("first callback did not start: %v", ctx.Err())
	}
	if err := second.wrapTask("shared-job", func(context.Context) error {
		executions.Add(1)
		return nil
	})(ctx); err != nil {
		t.Fatalf("second scheduler returned error: %v", err)
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("first scheduler returned error: %v", err)
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("expected exactly one execution while lease renewed, got %d", got)
	}
}

func TestWrapTaskRecordsExecutionOutcomeAndLeaseOwnership(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	store := newFakeLeaseStore()
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC"}, WithLeaseStore(store), WithTelemetry(recorder))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(context.Background()) })

	if err := scheduler.wrapTask("private-job-name", func(context.Context) error { return nil })(context.Background()); err != nil {
		t.Fatalf("wrapped task: %v", err)
	}
	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected scheduler count and duration, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["scheduler.operation"] != "run" || event.Attributes["scheduler.lease_owned"] != "true" || event.Attributes["scheduler.outcome"] != "ok" {
			t.Fatalf("scheduler telemetry mismatch: %+v", event)
		}
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(events)), "private-job-name") {
		t.Fatalf("scheduler job identity leaked: %+v", events)
	}
}

type fakeLeaseStore struct {
	mu                      sync.Mutex
	lease                   redislock.Lease
	acquireKey              string
	acquireTTL              time.Duration
	acquireErr              error
	renewErr                error
	renewCalls              int
	releaseCalls            int
	renewed                 chan struct{}
	renewStarted            chan struct{}
	blockRenewUntilCanceled bool
}

func newFakeLeaseStore() *fakeLeaseStore {
	return &fakeLeaseStore{lease: redislock.Lease{Token: 1}}
}

func (f *fakeLeaseStore) Acquire(_ context.Context, key string, owner string, ttl time.Duration) (redislock.Lease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireKey = key
	f.acquireTTL = ttl
	if f.acquireErr != nil {
		return redislock.Lease{}, f.acquireErr
	}
	f.lease.Key = key
	f.lease.Owner = owner
	f.lease.ExpiresAt = time.Now().Add(ttl)
	return f.lease, nil
}

func (f *fakeLeaseStore) Renew(ctx context.Context, lease redislock.Lease, ttl time.Duration) (redislock.Lease, error) {
	f.mu.Lock()
	f.renewCalls++
	err := f.renewErr
	renewed := f.renewed
	renewStarted := f.renewStarted
	blockUntilCanceled := f.blockRenewUntilCanceled
	f.mu.Unlock()
	if renewStarted != nil {
		select {
		case <-renewStarted:
		default:
			close(renewStarted)
		}
	}
	if blockUntilCanceled {
		<-ctx.Done()
		return redislock.Lease{}, ctx.Err()
	}
	if err != nil {
		return redislock.Lease{}, err
	}
	lease.ExpiresAt = time.Now().Add(ttl)
	if renewed != nil {
		select {
		case renewed <- struct{}{}:
		default:
		}
	}
	return lease, nil
}

func (f *fakeLeaseStore) Release(_ context.Context, _ redislock.Lease) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls++
	return nil
}

var _ redislock.LeaseStore = (*fakeLeaseStore)(nil)
