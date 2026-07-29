package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

var errAIObservabilityNotConfigured = errors.New("AI first-delta recorder is not configured")

type firstDeltaRecorder interface {
	MarkAttemptFirstDelta(context.Context, uint64, time.Time) (bool, error)
}

type firstDeliverableSink struct {
	next      infraai.EventSink
	recorder  firstDeltaRecorder
	attemptID uint64
	now       func() time.Time

	mu       sync.Mutex
	recorded bool
}

func newFirstDeliverableSink(next infraai.EventSink, recorder firstDeltaRecorder, attemptID uint64, now func() time.Time) *firstDeliverableSink {
	if now == nil {
		now = time.Now
	}
	return &firstDeliverableSink{next: next, recorder: recorder, attemptID: attemptID, now: now}
}

func (sink *firstDeliverableSink) Emit(ctx context.Context, event infraai.Event) error {
	if sink != nil && deliverableProviderEvent(event) {
		sink.mu.Lock()
		if !sink.recorded {
			if sink.recorder == nil || sink.attemptID == 0 {
				sink.mu.Unlock()
				return infraai.FatalEventSink(errAIObservabilityNotConfigured)
			}
			_, err := sink.recorder.MarkAttemptFirstDelta(ctx, sink.attemptID, sink.now().UTC())
			if err != nil {
				sink.mu.Unlock()
				return infraai.FatalEventSink(err)
			}
			sink.recorded = true
		}
		sink.mu.Unlock()
	}
	if sink == nil || sink.next == nil {
		return nil
	}
	return sink.next.Emit(ctx, event)
}

func deliverableProviderEvent(event infraai.Event) bool {
	switch strings.TrimSpace(event.Type) {
	case "delta":
		return strings.TrimSpace(event.DeltaText) != ""
	case "tool_delta":
		return nonEmptyEventString(event.Payload, "tool_call_id") ||
			nonEmptyEventString(event.Payload, "name") ||
			nonEmptyEventString(event.Payload, "arguments_delta")
	default:
		return false
	}
}

func nonEmptyEventString(payload map[string]any, key string) bool {
	value, ok := payload[key].(string)
	return ok && strings.TrimSpace(value) != ""
}
