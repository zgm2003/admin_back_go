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
