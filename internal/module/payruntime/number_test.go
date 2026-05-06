package payruntime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRedisNumberGeneratorFormatsLegacyCompatibleNumbers(t *testing.T) {
	generator := NewRedisNumberGenerator(fakeCounter{next: 7}, func() time.Time {
		return time.Date(2026, 5, 6, 12, 34, 56, 0, time.Local)
	})

	value, err := generator.Next(context.Background(), "R")
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if value != "R260506123456000007" {
		t.Fatalf("unexpected number: %q", value)
	}
}

func TestRedisNumberGeneratorRejectsInvalidPrefix(t *testing.T) {
	generator := NewRedisNumberGenerator(fakeCounter{next: 1}, fixedNow)

	if _, err := generator.Next(context.Background(), "X"); err == nil {
		t.Fatalf("expected invalid prefix error")
	}
}

func TestRedisNumberGeneratorWrapsCounterError(t *testing.T) {
	generator := NewRedisNumberGenerator(fakeCounter{err: errors.New("redis down")}, fixedNow)

	if _, err := generator.Next(context.Background(), "T"); err == nil {
		t.Fatalf("expected counter error")
	}
}

type fakeCounter struct {
	next int64
	err  error
}

func (c fakeCounter) Incr(ctx context.Context, key string) (int64, error) {
	if c.err != nil {
		return 0, c.err
	}
	return c.next, nil
}
