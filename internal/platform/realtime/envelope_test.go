package realtime

import (
	"encoding/json"
	"testing"
)

func TestNewEnvelopeEncodesObjectData(t *testing.T) {
	envelope, err := NewEnvelope("realtime.connected.v1", "rid-1", map[string]any{
		"user_id":  int64(7),
		"platform": "admin",
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	if envelope.Type != "realtime.connected.v1" || envelope.RequestID != "rid-1" {
		t.Fatalf("unexpected envelope metadata: %#v", envelope)
	}

	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("invalid data json: %v", err)
	}
	if data["platform"] != "admin" {
		t.Fatalf("expected platform admin, got %#v", data)
	}
}

func TestEncodeDecodeEnvelopeUsesEmptyObjectWhenDataMissing(t *testing.T) {
	encoded, err := EncodeEnvelope(Envelope{Type: "realtime.ping.v1"})
	if err != nil {
		t.Fatalf("EncodeEnvelope returned error: %v", err)
	}

	envelope, err := DecodeEnvelope(encoded)
	if err != nil {
		t.Fatalf("DecodeEnvelope returned error: %v", err)
	}
	if envelope.Type != "realtime.ping.v1" {
		t.Fatalf("unexpected type %s", envelope.Type)
	}
	if string(envelope.Data) != "{}" {
		t.Fatalf("expected empty object data, got %s", envelope.Data)
	}
}
