package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/platform/redislock"
)

func TestNewRejectsInvalidTimezone(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "bad/timezone"})
	if err == nil {
		t.Fatalf("expected invalid timezone to be rejected")
	}
	if scheduler != nil {
		t.Fatalf("expected nil scheduler on error")
	}
	if !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("expected ErrInvalidTimezone, got %v", err)
	}
}

func TestNewUsesConfiguredTimezone(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer scheduler.Shutdown(context.Background())

	if scheduler.location.String() != "Asia/Shanghai" {
		t.Fatalf("expected Asia/Shanghai location, got %s", scheduler.location)
	}
}

func TestEveryRejectsInvalidDefinition(t *testing.T) {
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer scheduler.Shutdown(context.Background())

	if err := scheduler.Every("", time.Minute, func(ctx context.Context) error { return nil }); !errors.Is(err, ErrJobNameRequired) {
		t.Fatalf("expected ErrJobNameRequired, got %v", err)
	}
	if err := scheduler.Every("job", 0, func(ctx context.Context) error { return nil }); !errors.Is(err, ErrJobIntervalRequired) {
		t.Fatalf("expected ErrJobIntervalRequired, got %v", err)
	}
	if err := scheduler.Every("job", time.Minute, nil); !errors.Is(err, ErrJobTaskRequired) {
		t.Fatalf("expected ErrJobTaskRequired, got %v", err)
	}
}

func TestWrapTaskUsesDistributedLockWhenConfigured(t *testing.T) {
	locker := &fakeLocker{}
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC", LockPrefix: "test:scheduler:", LockTTL: 45 * time.Second}, WithLocker(locker))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer scheduler.Shutdown(context.Background())

	run := false
	err = scheduler.wrapTask("job-a", func(ctx context.Context) error {
		run = true
		return nil
	})(context.Background())
	if err != nil {
		t.Fatalf("task returned error: %v", err)
	}
	if !run {
		t.Fatalf("expected task to run")
	}
	if locker.lockKey != "test:scheduler:job-a" {
		t.Fatalf("unexpected lock key: %q", locker.lockKey)
	}
	if locker.lockTTL != 45*time.Second {
		t.Fatalf("unexpected lock ttl: %s", locker.lockTTL)
	}
	if locker.unlockKey != "test:scheduler:job-a" || locker.unlockToken != "token" {
		t.Fatalf("unexpected unlock call: %#v", locker)
	}
}

func TestWrapTaskSkipsWhenDistributedLockNotAcquired(t *testing.T) {
	locker := &fakeLocker{lockErr: redislock.ErrNotAcquired}
	scheduler, err := New(config.SchedulerConfig{Timezone: "UTC", LockPrefix: "test:scheduler:", LockTTL: 30 * time.Second}, WithLocker(locker))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer scheduler.Shutdown(context.Background())

	run := false
	err = scheduler.wrapTask("job-a", func(ctx context.Context) error {
		run = true
		return nil
	})(context.Background())
	if err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
	if run {
		t.Fatalf("task should not run when lock is held")
	}
	if locker.unlockKey != "" {
		t.Fatalf("unlock should not be called when lock is not acquired")
	}
}

type fakeLocker struct {
	lockKey     string
	lockTTL     time.Duration
	lockErr     error
	unlockKey   string
	unlockToken string
	token       string
}

func (f *fakeLocker) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	f.lockKey = key
	f.lockTTL = ttl
	if f.lockErr != nil {
		return "", f.lockErr
	}
	if f.token == "" {
		f.token = "token"
	}
	return f.token, nil
}

func (f *fakeLocker) Unlock(ctx context.Context, key string, token string) error {
	f.unlockKey = key
	f.unlockToken = token
	return nil
}
