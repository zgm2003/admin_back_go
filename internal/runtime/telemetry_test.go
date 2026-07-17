package runtime

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/telemetry"
)

func TestProcessOptionsKeepOneRecorderAndSchedulerReconciliationUsesIt(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	settings := resolveProcessOptions([]ProcessOption{WithTelemetry(recorder)})
	if settings.recorder != recorder {
		t.Fatalf("runtime replaced provided recorder: %#v", settings.recorder)
	}

	reconcileErr := errors.New("reconcile failed")
	err := runSchedulerReconciliation(context.Background(), settings.recorder, func(context.Context) error {
		return reconcileErr
	})
	if !errors.Is(err, reconcileErr) {
		t.Fatalf("reconciliation error=%v", err)
	}
	events := recorder.Events()
	if len(events) != 2 || events[0].Kind != telemetry.EventStart || events[1].Kind != telemetry.EventEnd {
		t.Fatalf("scheduler reconciliation events=%+v", events)
	}
	for _, event := range events {
		if event.Name != "scheduler.reconciliation" || event.Attributes["scheduler.operation"] != "reconcile" {
			t.Fatalf("scheduler reconciliation attributes=%+v", event)
		}
	}
	if events[1].Attributes["outcome"] != "error" {
		t.Fatalf("scheduler error outcome missing: %+v", events[1])
	}
}

func TestProcessOptionsDefaultToNoopRecorder(t *testing.T) {
	settings := resolveProcessOptions(nil)
	if settings.recorder == nil {
		t.Fatal("runtime telemetry default must be non-nil")
	}
	settings.recorder.Count("ignored", 1, nil)
}
