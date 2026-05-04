package system

import (
	"context"
	"testing"

	"admin_back_go/internal/readiness"
)

type fakeReadinessChecker struct {
	report readiness.Report
}

func (f fakeReadinessChecker) Readiness(ctx context.Context) readiness.Report {
	return f.report
}

func TestServiceHealthReturnsRuntimeStatus(t *testing.T) {
	service := NewService(nil)

	got := service.Health()

	if got.Service != "admin-api" {
		t.Fatalf("expected service admin-api, got %q", got.Service)
	}
	if got.Status != "ok" {
		t.Fatalf("expected status ok, got %q", got.Status)
	}
	if got.Version == "" {
		t.Fatalf("expected version to be non-empty")
	}
}

func TestServicePingReturnsPong(t *testing.T) {
	service := NewService(nil)

	got := service.Ping()

	if got.Message != "pong" {
		t.Fatalf("expected message pong, got %q", got.Message)
	}
}

func TestServiceReadyUsesInjectedChecker(t *testing.T) {
	expected := readiness.NewReport(map[string]readiness.Check{
		"database":    {Status: readiness.StatusUp},
		"redis":       {Status: readiness.StatusDisabled},
		"token_redis": {Status: readiness.StatusDisabled},
		"queue_redis": {Status: readiness.StatusDisabled},
		"realtime":    {Status: readiness.StatusDisabled},
	})
	service := NewService(fakeReadinessChecker{report: expected})

	got := service.Ready(context.Background())

	if got.Status != readiness.StatusReady {
		t.Fatalf("expected ready status, got %q", got.Status)
	}
	if got.Checks["redis"].Status != readiness.StatusDisabled {
		t.Fatalf("expected redis disabled, got %#v", got.Checks["redis"])
	}
	if got.Checks["token_redis"].Status != readiness.StatusDisabled {
		t.Fatalf("expected token_redis disabled, got %#v", got.Checks["token_redis"])
	}
	if got.Checks["queue_redis"].Status != readiness.StatusDisabled {
		t.Fatalf("expected queue_redis disabled, got %#v", got.Checks["queue_redis"])
	}
	if got.Checks["realtime"].Status != readiness.StatusDisabled {
		t.Fatalf("expected realtime disabled, got %#v", got.Checks["realtime"])
	}
}

func TestServiceReadyDefaultsToDisabledResources(t *testing.T) {
	service := NewService(nil)

	got := service.Ready(context.Background())

	if got.Status != readiness.StatusReady {
		t.Fatalf("expected ready status, got %q", got.Status)
	}
	if got.Checks["database"].Status != readiness.StatusDisabled {
		t.Fatalf("expected database disabled, got %#v", got.Checks["database"])
	}
	if got.Checks["redis"].Status != readiness.StatusDisabled {
		t.Fatalf("expected redis disabled, got %#v", got.Checks["redis"])
	}
	if got.Checks["token_redis"].Status != readiness.StatusDisabled {
		t.Fatalf("expected token_redis disabled, got %#v", got.Checks["token_redis"])
	}
}
