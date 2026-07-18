package realtime

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
)

func TestEnvelopeRegistryRejectsUnknownAndMalformedPayloads(t *testing.T) {
	registry := DefaultRegistry()
	if _, err := registry.EncodePayload("server.guessed.v1", struct{}{}); !errors.Is(err, ErrUnknownEventType) {
		t.Fatalf("expected unknown event error, got %v", err)
	}
	if _, err := registry.DecodePayload(TypeSubscribeV1, json.RawMessage(`{"topics":["user:7"],"fallback":true}`)); !errors.Is(err, ErrEventPayloadInvalid) {
		t.Fatalf("expected closed subscribe payload error, got %v", err)
	}
	if _, err := registry.DecodePayload(TypeResumeV1, json.RawMessage(`{}`)); !errors.Is(err, ErrEventPayloadInvalid) {
		t.Fatalf("expected missing after_sequence error, got %v", err)
	}
}

func TestEnvelopeRegistryEnforcesDeclaredDurability(t *testing.T) {
	registry := DefaultRegistry()
	now := time.Date(2026, 7, 17, 11, 0, 0, 0, time.UTC)
	if _, err := registry.NewEphemeral(TypeNotificationCreatedV1, "rid", NotificationCreatedPayload{
		TaskID: 1, Title: "title", Content: "body", Link: "/", Level: "normal", NotificationType: "info",
	}, now); !errors.Is(err, ErrEventDurabilityInvalid) {
		t.Fatalf("durable notification was accepted as ephemeral: %v", err)
	}
	eventID, err := infrarealtime.NewEventID(now)
	if err != nil {
		t.Fatalf("NewEventID returned error: %v", err)
	}
	envelope, err := registry.NewDurable(eventID, TypeNotificationCreatedV1, "rid", 9, NotificationCreatedPayload{
		TaskID: 1, Title: "title", Content: "body", Link: "/", Level: "normal", NotificationType: "info",
	}, now)
	if err != nil {
		t.Fatalf("NewDurable returned error: %v", err)
	}
	if envelope.Durability != infrarealtime.Durable || envelope.Sequence != 9 {
		t.Fatalf("unexpected durable envelope: %#v", envelope)
	}
	if _, err := registry.NewDurable(eventID, TypeAIResponseDeltaV1, "rid", 10, AIResponseDeltaPayload{
		ConversationID: 3, RequestID: "rid", Delta: "hello",
	}, now); !errors.Is(err, ErrEventDurabilityInvalid) {
		t.Fatalf("ephemeral delta was accepted as durable: %v", err)
	}
	if _, err := registry.NewEphemeral(TypeSubscribeV1, "rid", SubscribePayload{Topics: []string{"user:7"}}, now); err == nil {
		t.Fatalf("client-only event was accepted as a server envelope: %v", err)
	}
}

func TestEnvelopeRegistryRoundTripsTypedPayload(t *testing.T) {
	registry := DefaultRegistry()
	raw, err := registry.EncodePayload(TypeAIResponseCompletedV1, AIResponseCompletedPayload{
		ConversationID: 3, RequestID: "request-1", AssistantMessageID: 11,
	})
	if err != nil {
		t.Fatalf("EncodePayload returned error: %v", err)
	}
	decoded, err := registry.DecodePayload(TypeAIResponseCompletedV1, raw)
	if err != nil {
		t.Fatalf("DecodePayload returned error: %v", err)
	}
	payload, ok := decoded.(*AIResponseCompletedPayload)
	if !ok || payload.ConversationID != 3 || payload.AssistantMessageID != 11 {
		t.Fatalf("unexpected decoded payload: %#v", decoded)
	}
}

func TestConfirmedRecoveryEventsUseExactPayloadAndDurability(t *testing.T) {
	registry := DefaultRegistry()
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)

	resync, err := registry.NewEphemeral(TypeResyncRequiredV1, "resume-1", ResyncRequiredPayload{LatestSequence: 123}, now)
	if err != nil {
		t.Fatalf("build resync event: %v", err)
	}
	if resync.Durability != infrarealtime.Ephemeral || resync.Sequence != 0 || string(resync.Data) != `{"latest_sequence":123}` {
		t.Fatalf("unexpected resync envelope: %#v data=%s", resync, resync.Data)
	}
	if _, err := registry.DecodePayload(TypeResyncRequiredV1, json.RawMessage(`{"latest_sequence":123,"fallback":true}`)); !errors.Is(err, ErrEventPayloadInvalid) {
		t.Fatalf("resync payload accepted undocumented field: %v", err)
	}

	eventID, err := infrarealtime.NewEventID(now)
	if err != nil {
		t.Fatalf("create canceled event id: %v", err)
	}
	canceled, err := registry.NewDurable(eventID, TypeAIResponseCanceledV1, "request-1", 9, AIResponseCanceledPayload{
		ConversationID: 3,
		RequestID:      "request-1",
	}, now)
	if err != nil {
		t.Fatalf("build canceled event: %v", err)
	}
	if canceled.Durability != infrarealtime.Durable || string(canceled.Data) != `{"conversation_id":3,"request_id":"request-1"}` {
		t.Fatalf("unexpected canceled envelope: %#v data=%s", canceled, canceled.Data)
	}
}

func TestEnvelopeRegistryUsesJSONCharacterLengths(t *testing.T) {
	registry := DefaultRegistry()
	requestID := strings.Repeat("界", 128)
	if _, err := registry.EncodePayload(TypeAIResponseStartV1, AIResponseStartPayload{
		ConversationID: 3,
		RequestID:      requestID,
		UserMessageID:  9,
		AgentID:        2,
	}); err != nil {
		t.Fatalf("128-character request_id must satisfy the schema: %v", err)
	}
	if _, err := registry.EncodePayload(TypeSubscribeV1, SubscribePayload{Topics: []string{strings.Repeat("界", 129)}}); !errors.Is(err, ErrEventPayloadInvalid) {
		t.Fatalf("129-character topic must violate the schema: %v", err)
	}
}

func TestEnvelopeRegistryDefinitionsAreStableAndSorted(t *testing.T) {
	definitions := DefaultRegistry().Definitions()
	types := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		types = append(types, definition.Type)
	}
	if !sort.StringsAreSorted(types) {
		t.Fatalf("registry definitions must be deterministic for contract generation: %v", types)
	}
}
