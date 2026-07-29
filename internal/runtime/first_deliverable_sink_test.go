package runtime

import (
	"context"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

type fakeFirstDeltaRecorder struct {
	calls     int
	attemptID uint64
	at        time.Time
	err       error
}

func (recorder *fakeFirstDeltaRecorder) MarkAttemptFirstDelta(_ context.Context, attemptID uint64, at time.Time) (bool, error) {
	recorder.calls++
	recorder.attemptID, recorder.at = attemptID, at
	return true, recorder.err
}

type collectingEventSink struct{ events []infraai.Event }

func (sink *collectingEventSink) Emit(_ context.Context, event infraai.Event) error {
	sink.events = append(sink.events, event)
	return nil
}

func TestFirstDeliverableSinkIgnoresEmptyAndUsageEvents(t *testing.T) {
	recorder := &fakeFirstDeltaRecorder{}
	downstream := &collectingEventSink{}
	sink := newFirstDeliverableSink(downstream, recorder, 71, func() time.Time {
		return time.Date(2026, 7, 28, 11, 0, 0, 0, time.UTC)
	})
	events := []infraai.Event{
		{Type: "delta", DeltaText: "  "},
		{Type: "tool_delta", Payload: map[string]any{}},
		{Type: "usage", Payload: map[string]any{"total_tokens": 2}},
	}
	for _, event := range events {
		if err := sink.Emit(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if recorder.calls != 0 || len(downstream.events) != len(events) {
		t.Fatalf("recorder calls=%d downstream=%d", recorder.calls, len(downstream.events))
	}
}

func TestFirstDeliverableSinkRecordsTextOrToolDeltaOnce(t *testing.T) {
	for _, test := range []struct {
		name  string
		event infraai.Event
	}{
		{name: "text", event: infraai.Event{Type: "delta", DeltaText: "hi"}},
		{name: "tool", event: infraai.Event{Type: "tool_delta", Payload: map[string]any{"arguments_delta": "{"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 7, 28, 11, 0, 1, 123456000, time.UTC)
			recorder := &fakeFirstDeltaRecorder{}
			sink := newFirstDeliverableSink(&collectingEventSink{}, recorder, 72, func() time.Time { return now })
			if err := sink.Emit(context.Background(), test.event); err != nil {
				t.Fatal(err)
			}
			if err := sink.Emit(context.Background(), infraai.Event{Type: "delta", DeltaText: "second"}); err != nil {
				t.Fatal(err)
			}
			if recorder.calls != 1 || recorder.attemptID != 72 || !recorder.at.Equal(now) {
				t.Fatalf("recorder=%+v", recorder)
			}
		})
	}
}
