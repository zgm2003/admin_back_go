package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/config"
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
