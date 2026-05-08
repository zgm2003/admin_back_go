package aichat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStreamEventIDsAreMonotonic(t *testing.T) {
	gen := NewStreamIDGenerator()
	first := gen.Next()
	second := gen.Next()
	if !isNewerStreamID(second, first) {
		t.Fatalf("expected second id to be newer: first=%s second=%s", first, second)
	}
}

func TestEnvelopeBuildersUseVersionedAIResponseTypes(t *testing.T) {
	cases := []struct {
		name         string
		envelopeType string
		build        func() (EnvelopeEvent, error)
	}{
		{"start", EventAIResponseStart, func() (EnvelopeEvent, error) {
			return BuildStartEvent(StartPayload{RunID: 7, ConversationID: 3, RequestID: "rid", UserMessageID: 9, AgentID: 2})
		}},
		{"delta", EventAIResponseDelta, func() (EnvelopeEvent, error) { return BuildDeltaEvent(7, "hello") }},
		{"completed", EventAIResponseCompleted, func() (EnvelopeEvent, error) {
			return BuildCompletedEvent(CompletedPayload{RunID: 7, ConversationID: 3, UserMessageID: 9, AssistantMessageID: 10})
		}},
		{"failed", EventAIResponseFailed, func() (EnvelopeEvent, error) { return BuildFailedEvent(7, "bad") }},
		{"cancel", EventAIResponseCancel, func() (EnvelopeEvent, error) { return BuildCancelEvent(7) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := tc.build()
			if err != nil {
				t.Fatalf("builder returned error: %v", err)
			}
			if event.Envelope.Type != tc.envelopeType {
				t.Fatalf("expected type %s, got %#v", tc.envelopeType, event.Envelope)
			}
			if strings.Contains(event.Envelope.Type, "ai_run"+"_event") {
				t.Fatalf("builder emitted legacy event type: %s", event.Envelope.Type)
			}
			var data map[string]any
			if err := json.Unmarshal(event.Envelope.Data, &data); err != nil {
				t.Fatalf("invalid data: %v", err)
			}
			if data["run_id"] != float64(7) {
				t.Fatalf("expected run_id in envelope data, got %#v", data)
			}
		})
	}
}
