package aichat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvelopeBuildersUseConversationScopedPayloads(t *testing.T) {
	cases := []struct {
		name         string
		envelopeType string
		build        func() (EnvelopeEvent, error)
	}{
		{"start", EventAIResponseStart, func() (EnvelopeEvent, error) {
			return BuildStartEvent(StartPayload{ConversationID: 3, RequestID: "rid", UserMessageID: 9, AgentID: 2})
		}},
		{"delta", EventAIResponseDelta, func() (EnvelopeEvent, error) {
			return BuildDeltaEvent(DeltaPayload{ConversationID: 3, RequestID: "rid", Delta: "hello"})
		}},
		{"completed", EventAIResponseCompleted, func() (EnvelopeEvent, error) {
			return BuildCompletedEvent(CompletedPayload{ConversationID: 3, RequestID: "rid", AssistantMessageID: 10})
		}},
		{"failed", EventAIResponseFailed, func() (EnvelopeEvent, error) {
			return BuildFailedEvent(FailedPayload{ConversationID: 3, RequestID: "rid", Msg: "bad"})
		}},
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
			var data map[string]any
			if err := json.Unmarshal(event.Envelope.Data, &data); err != nil {
				t.Fatalf("invalid data: %v", err)
			}
			if _, ok := data["run_id"]; ok || strings.Contains(string(event.Envelope.Data), "run_id") {
				t.Fatalf("conversation event must not contain run_id: %s", string(event.Envelope.Data))
			}
			if data["conversation_id"] != float64(3) || data["request_id"] != "rid" {
				t.Fatalf("unexpected payload: %#v", data)
			}
		})
	}
}
