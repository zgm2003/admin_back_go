package payment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRedisNumberGeneratorFormatsPaymentOrderNo(t *testing.T) {
	generator := NewRedisNumberGenerator(fakeCounter{next: 7}, func() time.Time {
		return time.Date(2026, 5, 8, 12, 34, 56, 0, time.UTC)
	})

	value, err := generator.Next(context.Background(), "P")
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if value != "P260508123456000007" {
		t.Fatalf("unexpected number: %q", value)
	}
}

func TestRedisNumberGeneratorRejectsInvalidPaymentPrefix(t *testing.T) {
	generator := NewRedisNumberGenerator(fakeCounter{next: 1}, fixedNow)

	if _, err := generator.Next(context.Background(), "R"); err == nil {
		t.Fatalf("expected invalid prefix error")
	}
}

func TestRedisNumberGeneratorWrapsPaymentCounterError(t *testing.T) {
	generator := NewRedisNumberGenerator(fakeCounter{err: errors.New("redis down")}, fixedNow)

	_, err := generator.Next(context.Background(), "P")
	if err == nil {
		t.Fatalf("expected counter error")
	}
	if !strings.Contains(err.Error(), "redis down") {
		t.Fatalf("counter error was not wrapped: %v", err)
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

func fixedNow() time.Time {
	return time.Date(2026, 5, 8, 12, 34, 56, 0, time.UTC)
}
