package redeemcode

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/telemetry"
)

func TestTelemetryRecordsControlledRedeemCodeMetrics(t *testing.T) {
	memory := telemetry.NewMemoryRecorder()
	metrics := newMetrics(memory)
	metrics.batch("ok", "created")
	metrics.codes(3, StateUnused)
	metrics.transition(2, StateVoided, "admin_void")
	metrics.redemption("ok", "created", 250*time.Millisecond)
	metrics.conflict("generate", "request")

	events := memory.Events()
	wantNames := []string{
		metricBatches,
		metricCodes,
		metricStateTransitions,
		metricRedemptions,
		metricRedemptionLatency,
		metricConflicts,
	}
	if len(events) != len(wantNames) {
		t.Fatalf("events=%+v", events)
	}
	for index, want := range wantNames {
		if events[index].Name != want {
			t.Fatalf("event[%d].Name=%q want %q", index, events[index].Name, want)
		}
		for key := range events[index].Attributes {
			if key != "operation" && key != "outcome" && key != "state" && key != "reason" {
				t.Fatalf("event[%d] retained forbidden attribute %q", index, key)
			}
		}
	}
	if events[1].Value != 3 || events[2].Value != 2 || events[4].Value != 0.25 {
		t.Fatalf("metric values=%+v", events)
	}
}

func TestTelemetryPassesOnlyBoundedAttributeNamesAndReasons(t *testing.T) {
	recorder := &captureRecorder{}
	metrics := newMetrics(recorder)
	metrics.redemption("error", "wallet_overflow", time.Second)
	metrics.conflict("redeem", "source_unique")

	if len(recorder.events) != 3 {
		t.Fatalf("events=%+v", recorder.events)
	}
	for _, event := range recorder.events {
		for key := range event.attributes {
			if key != "operation" && key != "outcome" && key != "state" && key != "reason" {
				t.Fatalf("forbidden telemetry attribute %q", key)
			}
		}
		if _, found := event.attributes["user_id"]; found {
			t.Fatal("telemetry must not include user identity")
		}
	}
	if recorder.events[0].attributes["reason"] != "wallet_overflow" || recorder.events[2].attributes["reason"] != "source_unique" {
		t.Fatalf("controlled reasons=%+v", recorder.events)
	}
}

type capturedMetric struct {
	name       string
	attributes telemetry.Attributes
}

type captureRecorder struct{ events []capturedMetric }

func (recorder *captureRecorder) Count(name string, _ float64, attributes telemetry.Attributes) {
	recorder.events = append(recorder.events, capturedMetric{name: name, attributes: attributes})
}

func (recorder *captureRecorder) Observe(name string, _ float64, attributes telemetry.Attributes) {
	recorder.events = append(recorder.events, capturedMetric{name: name, attributes: attributes})
}

func (recorder *captureRecorder) Start(ctx context.Context, _ string, _ telemetry.Attributes) (context.Context, func(error)) {
	return ctx, func(error) {}
}
