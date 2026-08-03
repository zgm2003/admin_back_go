package readiness

import "testing"

func TestNewReportIsReadyWhenAllChecksAreUpOrDisabled(t *testing.T) {
	report := NewReport(map[string]Check{
		"database": {Status: StatusUp},
		"redis":    {Status: StatusDisabled},
	})

	if report.Status != StatusReady {
		t.Fatalf("expected ready status, got %q", report.Status)
	}
}

func TestNewReportIsNotReadyWhenAnyCheckIsDown(t *testing.T) {
	report := NewReport(map[string]Check{
		"database": {Status: StatusUp},
		"redis":    {Status: StatusDown, Message: "connection refused"},
	})

	if report.Status != StatusNotReady {
		t.Fatalf("expected not_ready status, got %q", report.Status)
	}
	if report.Checks["redis"].Message != "connection refused" {
		t.Fatalf("expected redis failure message, got %#v", report.Checks["redis"])
	}
}

func TestReadinessReportKeepsDegradedVisibleButNonBlocking(t *testing.T) {
	report := NewReport(map[string]Check{
		"database": {Status: StatusUp},
		"qdrant":   {Status: StatusDegraded, Message: "context index is unavailable"},
	})

	if report.Status != StatusReady {
		t.Fatalf("degraded component must not block readiness, got %#v", report)
	}
	if report.Checks["qdrant"].Status != StatusDegraded {
		t.Fatalf("degraded component must remain visible, got %#v", report.Checks["qdrant"])
	}
}
