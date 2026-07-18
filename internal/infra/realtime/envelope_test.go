package realtime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEnvelopeNewEphemeralEnvelopeCarriesClosedMetadata(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 11, 12, 123456000, time.UTC)
	envelope, err := NewEnvelopeAt("realtime.connected.v1", "rid-1", map[string]any{
		"user_id":  int64(7),
		"platform": "admin",
	}, now)
	if err != nil {
		t.Fatalf("NewEnvelopeAt returned error: %v", err)
	}
	if envelope.Type != "realtime.connected.v1" || envelope.RequestID != "rid-1" {
		t.Fatalf("unexpected envelope metadata: %#v", envelope)
	}
	if !ValidEventID(envelope.EventID) {
		t.Fatalf("event_id is not ULID-compatible: %q", envelope.EventID)
	}
	if envelope.Sequence != 0 || envelope.Durability != Ephemeral || !envelope.OccurredAt.Equal(now) {
		t.Fatalf("unexpected ephemeral metadata: %#v", envelope)
	}

	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("invalid data json: %v", err)
	}
	if data["platform"] != "admin" {
		t.Fatalf("expected platform admin, got %#v", data)
	}
}

func TestEnvelopeDurableEnvelopeRequiresSequenceAndValidEventID(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 11, 12, 0, time.UTC)
	eventID, err := NewEventID(now)
	if err != nil {
		t.Fatalf("NewEventID returned error: %v", err)
	}
	if _, err := NewDurableEnvelope(eventID, "notification.created.v1", "rid", 0, now, map[string]any{}); err == nil {
		t.Fatal("expected zero durable sequence to fail")
	}
	if _, err := NewDurableEnvelope("not-an-event-id", "notification.created.v1", "rid", 1, now, map[string]any{}); err == nil {
		t.Fatal("expected invalid durable event_id to fail")
	}
	envelope, err := NewDurableEnvelope(eventID, "notification.created.v1", "rid", 41, now, map[string]any{"task_id": 9})
	if err != nil {
		t.Fatalf("NewDurableEnvelope returned error: %v", err)
	}
	if envelope.Sequence != 41 || envelope.Durability != Durable || envelope.EventID != eventID {
		t.Fatalf("unexpected durable envelope: %#v", envelope)
	}
	if _, err := EncodeEnvelope(envelope); err != nil {
		t.Fatalf("EncodeEnvelope rejected valid durable envelope: %v", err)
	}
}

func TestEnvelopeDecodeAcceptsMetadataFreeClientControlAndRejectsUnknownField(t *testing.T) {
	envelope, err := DecodeEnvelope([]byte(`{"type":"realtime.ping.v1","request_id":"rid","data":{}}`))
	if err != nil {
		t.Fatalf("DecodeEnvelope rejected client control: %v", err)
	}
	if envelope.Type != "realtime.ping.v1" || string(envelope.Data) != "{}" {
		t.Fatalf("unexpected decoded client envelope: %#v", envelope)
	}
	if _, err := DecodeEnvelope([]byte(`{"type":"realtime.ping.v1","data":{},"guess":true}`)); err == nil {
		t.Fatal("expected unknown top-level field to fail")
	}
	if _, err := DecodeEnvelope([]byte(`{"type":"realtime.ping.v1","data":{}} trailing`)); err == nil {
		t.Fatal("expected trailing non-JSON content to fail")
	}
	if _, err := DecodeEnvelope([]byte(`{"type":"realtime.ping.v1","request_id":"rid"}`)); err == nil {
		t.Fatal("missing data must not be replaced with an empty object")
	}
}

func TestEnvelopeEncodeRejectsMissingDataInsteadOfDefaultingIt(t *testing.T) {
	envelope, err := NewEnvelope("realtime.ping.v1", "rid", map[string]any{})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	envelope.Data = nil
	if _, err := EncodeEnvelope(envelope); err == nil {
		t.Fatal("missing server data must not be replaced with an empty object")
	}
}

func TestEnvelopeDecodeRejectsServerOwnedMetadataFromClient(t *testing.T) {
	serverOwnedFields := []string{
		`"event_id":"01K0ABCDEF0123456789ABCDEFG"`,
		`"sequence":0`,
		`"occurred_at":"2026-07-17T10:11:12Z"`,
		`"durability":"ephemeral"`,
	}
	for _, field := range serverOwnedFields {
		payload := []byte(`{"type":"realtime.ping.v1","request_id":"rid","data":{},` + field + `}`)
		if _, err := DecodeEnvelope(payload); err == nil {
			t.Fatalf("client envelope accepted server-owned field %s", field)
		}
	}
}

func TestEnvelopeEventIDsAreUniqueAndUseCrockfordAlphabet(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 11, 12, 0, time.UTC)
	seen := make(map[string]struct{}, 128)
	for range 128 {
		id, err := NewEventID(now)
		if err != nil {
			t.Fatalf("NewEventID returned error: %v", err)
		}
		if !ValidEventID(id) || strings.ContainsAny(id, "ILOU") {
			t.Fatalf("invalid Crockford event ID %q", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate event ID %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestEnvelopeRequestIDUsesSchemaCharacterLimit(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 11, 12, 0, time.UTC)
	if _, err := NewEnvelopeAt("realtime.ping.v1", strings.Repeat("界", 128), map[string]any{}, now); err != nil {
		t.Fatalf("128-character request_id must satisfy the schema: %v", err)
	}
	if _, err := NewEnvelopeAt("realtime.ping.v1", strings.Repeat("界", 129), map[string]any{}, now); err == nil {
		t.Fatal("129-character server request_id must violate the schema")
	}
	clientPayload, err := json.Marshal(map[string]any{
		"type":       "realtime.ping.v1",
		"request_id": strings.Repeat("界", 129),
		"data":       map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal client envelope: %v", err)
	}
	if _, err := DecodeEnvelope(clientPayload); err == nil {
		t.Fatal("129-character client request_id must violate the schema")
	}
}
