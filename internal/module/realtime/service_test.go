package realtime

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/middleware"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

func TestConnectedEnvelopeIncludesIdentityAndHeartbeat(t *testing.T) {
	service := NewService(25 * time.Second)

	envelope, err := service.ConnectedEnvelope(&middleware.AuthIdentity{
		UserID:    7,
		SessionID: 99,
		Platform:  "admin",
	}, "rid-1")
	if err != nil {
		t.Fatalf("ConnectedEnvelope returned error: %v", err)
	}
	if envelope.Type != TypeConnectedV1 || envelope.RequestID != "rid-1" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}

	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("invalid data: %v", err)
	}
	if data["platform"] != "admin" || data["heartbeat_interval_ms"] != float64(25000) {
		t.Fatalf("unexpected data: %#v", data)
	}
}

func TestSessionKeyBindsPlatformUserAndSession(t *testing.T) {
	service := NewService(time.Second)

	key := service.SessionKey(&middleware.AuthIdentity{
		UserID:    7,
		SessionID: 9,
		Platform:  "admin",
	})

	if key != "admin:7:9" {
		t.Fatalf("unexpected session key: %s", key)
	}
}

func TestHandleClientEnvelopeRepliesToPing(t *testing.T) {
	service := NewService(time.Second)
	service.now = func() time.Time {
		return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	}

	reply, err := service.HandleClientEnvelope(&middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, platformrealtime.Envelope{
		Type:      TypePingV1,
		RequestID: "rid-1",
		Data:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if reply == nil || reply.Type != TypePongV1 || reply.RequestID != "rid-1" {
		t.Fatalf("unexpected reply: %#v", reply)
	}
}

func TestHandleClientEnvelopeSubscribesOnlyAllowedIdentityTopics(t *testing.T) {
	service := NewService(time.Second)

	reply, err := service.HandleClientEnvelope(&middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, platformrealtime.Envelope{
		Type:      TypeSubscribeV1,
		RequestID: "rid-subscribe",
		Data:      json.RawMessage(`{"topics":["user:7","session:9","platform:admin"]}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if reply == nil || reply.Type != TypeSubscribedV1 || reply.RequestID != "rid-subscribe" {
		t.Fatalf("unexpected subscribe reply: %#v", reply)
	}

	var data map[string][]string
	if err := json.Unmarshal(reply.Data, &data); err != nil {
		t.Fatalf("invalid subscribe data: %v", err)
	}
	if got := data["topics"]; !reflect.DeepEqual(got, []string{"user:7", "session:9", "platform:admin"}) {
		t.Fatalf("unexpected subscribed topics: %#v", got)
	}
}

func TestHandleClientEnvelopeRejectsUnauthorizedTopic(t *testing.T) {
	service := NewService(time.Second)

	reply, err := service.HandleClientEnvelope(&middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, platformrealtime.Envelope{
		Type:      TypeSubscribeV1,
		RequestID: "rid-subscribe",
		Data:      json.RawMessage(`{"topics":["user:8"]}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if reply == nil || reply.Type != TypeErrorV1 || reply.RequestID != "rid-subscribe" {
		t.Fatalf("expected error reply, got %#v", reply)
	}

	var data map[string]any
	if err := json.Unmarshal(reply.Data, &data); err != nil {
		t.Fatalf("invalid error data: %v", err)
	}
	if data["code"] != float64(403) {
		t.Fatalf("expected unauthorized topic code 403, got %#v", data)
	}
}

func TestHandleClientEnvelopeRejectsUnsupportedType(t *testing.T) {
	service := NewService(time.Second)

	reply, err := service.HandleClientEnvelope(&middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, platformrealtime.Envelope{
		Type:      "client.unknown.v1",
		RequestID: "rid-1",
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if reply == nil || reply.Type != TypeErrorV1 {
		t.Fatalf("expected error reply, got %#v", reply)
	}
}
