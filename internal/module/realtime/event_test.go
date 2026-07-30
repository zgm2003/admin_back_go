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
	if _, err := registry.NewDurable(eventID, TypeAIResponseDeltaV2, "rid", 10, AIResponseDeltaPayload{
		ConversationID: 3, RequestID: "rid", DeliverySeq: 1, Delta: "hello",
	}, now); !errors.Is(err, ErrEventDurabilityInvalid) {
		t.Fatalf("ephemeral delta was accepted as durable: %v", err)
	}
	if _, err := registry.NewEphemeral(TypeSubscribeV1, "rid", SubscribePayload{Topics: []string{"user:7"}}, now); err == nil {
		t.Fatalf("client-only event was accepted as a server envelope: %v", err)
	}
}

func TestAIResponseDeltaV2RequiresContinuousDeliveryIdentity(t *testing.T) {
	registry := DefaultRegistry()
	definition, ok := registry.Definition(TypeAIResponseDeltaV2)
	if !ok || definition.Durability != infrarealtime.Ephemeral || definition.Direction != DirectionServer {
		t.Fatalf("definition=%+v ok=%v", definition, ok)
	}
	if _, exists := registry.Definition("ai.response.delta.v1"); exists {
		t.Fatal("delta v1 must not remain in the runtime registry")
	}

	envelope, err := registry.NewEphemeral(TypeAIResponseDeltaV2, "request-1", AIResponseDeltaPayload{
		ConversationID: 3,
		RequestID:      "request-1",
		DeliverySeq:    7,
		Delta:          "  你\n",
	}, time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(envelope.Data); got != `{"conversation_id":3,"request_id":"request-1","delivery_seq":7,"delta":"  你\n"}` {
		t.Fatalf("payload=%s", got)
	}

	invalid := []AIResponseDeltaPayload{
		{ConversationID: 3, RequestID: "request-1", DeliverySeq: 0, Delta: "x"},
		{ConversationID: 3, RequestID: "request-1", DeliverySeq: 1, Delta: ""},
		{ConversationID: 3, RequestID: "request-1", DeliverySeq: 1, Delta: strings.Repeat("界", 5462)},
		{ConversationID: 3, RequestID: "request-1", DeliverySeq: 1, Delta: string([]byte{0xff})},
	}
	for _, payload := range invalid {
		if err := payload.Validate(); err == nil {
			t.Fatalf("invalid payload accepted: seq=%d bytes=%d", payload.DeliverySeq, len(payload.Delta))
		}
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
	canceled, err := registry.NewDurable(eventID, TypeAIResponseCanceledV2, "request-1", 9, AIResponseCanceledPayload{
		ConversationID:     3,
		RequestID:          "request-1",
		AssistantMessageID: 97,
	}, now)
	if err != nil {
		t.Fatalf("build canceled event: %v", err)
	}
	if canceled.Durability != infrarealtime.Durable || string(canceled.Data) != `{"conversation_id":3,"request_id":"request-1","assistant_message_id":97}` {
		t.Fatalf("unexpected canceled envelope: %#v data=%s", canceled, canceled.Data)
	}
	if _, exists := registry.Definition("ai.response.canceled.v1"); exists {
		t.Fatal("canceled v1 must not remain in the runtime registry")
	}
	if err := (&AIResponseCanceledPayload{ConversationID: 3, RequestID: "request-1"}).Validate(); err == nil {
		t.Fatal("canceled payload without stopped assistant message was accepted")
	}
}

func TestAIResponseFailedPayloadRequiresMachineCodeAndExplicitWalletPaths(t *testing.T) {
	registry := DefaultRegistry()
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	eventID, err := infrarealtime.NewEventID(now)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := registry.NewDurable(eventID, TypeAIResponseFailedV1, "request-1", 1, AIResponseFailedPayload{
		ConversationID: 3, RequestID: "request-1", Msg: "failed",
	}, now); !errors.Is(err, ErrEventPayloadInvalid) {
		t.Fatalf("blank machine code accepted: %v", err)
	}
	if _, err := registry.NewDurable(eventID, TypeAIResponseFailedV1, "request-1", 1, AIResponseFailedPayload{
		ConversationID: 3, RequestID: "request-1", Msg: "failed", ErrorCode: " ai.reply_failed ",
	}, now); !errors.Is(err, ErrEventPayloadInvalid) {
		t.Fatalf("machine code with surrounding whitespace accepted: %v", err)
	}

	walletPath := "/profile/wallet"
	rechargePath := "/payment/recharge"
	envelope, err := registry.NewDurable(eventID, TypeAIResponseFailedV1, "request-1", 1, AIResponseFailedPayload{
		ConversationID: 3,
		RequestID:      "request-1",
		Msg:            "balance is insufficient",
		ErrorCode:      "ai.billing.insufficient_balance",
		WalletPath:     &walletPath,
		RechargePath:   &rechargePath,
	}, now)
	if err != nil {
		t.Fatalf("valid billing failure rejected: %v", err)
	}
	if got := string(envelope.Data); got != `{"conversation_id":3,"request_id":"request-1","msg":"balance is insufficient","error_code":"ai.billing.insufficient_balance","wallet_path":"/profile/wallet","recharge_path":"/payment/recharge"}` {
		t.Fatalf("billing failure payload=%s", got)
	}

	nonBilling, err := registry.NewDurable(eventID, TypeAIResponseFailedV1, "request-2", 2, AIResponseFailedPayload{
		ConversationID: 3, RequestID: "request-2", Msg: "provider failed", ErrorCode: "ai.reply_failed",
	}, now)
	if err != nil {
		t.Fatalf("valid non-billing failure rejected: %v", err)
	}
	if got := string(nonBilling.Data); got != `{"conversation_id":3,"request_id":"request-2","msg":"provider failed","error_code":"ai.reply_failed","wallet_path":null,"recharge_path":null}` {
		t.Fatalf("non-billing failure payload=%s", got)
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
