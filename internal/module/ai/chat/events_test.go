package aichat

import (
	"encoding/json"
	"strings"
	"testing"

	infrarealtime "admin_back_go/internal/infra/realtime"
)

func TestEnvelopeBuildersUseConversationScopedPayloads(t *testing.T) {
	cases := []struct {
		name         string
		envelopeType string
		build        func() (infrarealtime.Envelope, error)
	}{
		{"start", EventAIResponseStart, func() (infrarealtime.Envelope, error) {
			return BuildStartEvent(StartPayload{ConversationID: 3, RequestID: "rid", UserMessageID: 9, AgentID: 2})
		}},
		{"delta", EventAIResponseDelta, func() (infrarealtime.Envelope, error) {
			return BuildDeltaEvent(DeltaPayload{ConversationID: 3, RequestID: "rid", DeliverySeq: 1, Delta: "hello"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := tc.build()
			if err != nil {
				t.Fatalf("builder returned error: %v", err)
			}
			if event.Type != tc.envelopeType {
				t.Fatalf("expected type %s, got %#v", tc.envelopeType, event)
			}
			if event.RequestID != "rid" {
				t.Fatalf("envelope request_id must preserve the documented correlation ID: %#v", event)
			}
			var data map[string]any
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatalf("invalid data: %v", err)
			}
			if _, ok := data["run_id"]; ok || strings.Contains(string(event.Data), "run_id") {
				t.Fatalf("conversation event must not contain run_id: %s", string(event.Data))
			}
			if data["conversation_id"] != float64(3) || data["request_id"] != "rid" {
				t.Fatalf("unexpected payload: %#v", data)
			}
		})
	}
}

func TestAIResponseEventAliasesUseDeliveryV2(t *testing.T) {
	if EventAIResponseDelta != "ai.response.delta.v2" || EventAIResponseCanceled != "ai.response.canceled.v2" {
		t.Fatalf("delta=%q canceled=%q", EventAIResponseDelta, EventAIResponseCanceled)
	}
}
