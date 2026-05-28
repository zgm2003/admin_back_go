package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestEncodeDecodeRedisPublicationPreservesTargetAndEnvelope(t *testing.T) {
	envelope, err := NewEnvelope("notification.created.v1", "rid-redis", map[string]any{"level": "urgent"})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}

	payload, err := encodeRedisPublication(Publication{Platform: "admin", UserID: 7, Envelope: envelope})
	if err != nil {
		t.Fatalf("encodeRedisPublication returned error: %v", err)
	}
	got, err := decodeRedisPublication(payload)
	if err != nil {
		t.Fatalf("decodeRedisPublication returned error: %v", err)
	}
	if got.Platform != "admin" || got.UserID != 7 || got.Envelope.Type != "notification.created.v1" || got.Envelope.RequestID != "rid-redis" {
		t.Fatalf("unexpected decoded publication: %#v", got)
	}
	var data map[string]string
	if err := json.Unmarshal(got.Envelope.Data, &data); err != nil {
		t.Fatalf("invalid envelope data: %v", err)
	}
	if data["level"] != "urgent" {
		t.Fatalf("unexpected envelope data: %#v", data)
	}
}

func TestRedisPublisherRejectsMissingTargetBeforePublish(t *testing.T) {
	publisher := NewRedisPublisher(nil, "admin_go:realtime:test")
	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid")

	err := publisher.Publish(context.Background(), Publication{Envelope: envelope})
	if !errors.Is(err, ErrPublicationTargetRequired) {
		t.Fatalf("expected ErrPublicationTargetRequired, got %v", err)
	}
}

func TestRedisSubscriberHandlePayloadDeliversLocally(t *testing.T) {
	manager := NewManager()
	session := NewSession(nil, SessionOptions{SendBuffer: 1})
	defer session.Close()
	manager.Register("admin:7:1", session)
	local := NewLocalPublisher(manager)
	subscriber := NewRedisSubscriber(nil, "admin_go:realtime:test", local)
	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid")
	payload, err := encodeRedisPublication(Publication{Platform: "admin", UserID: 7, Envelope: envelope})
	if err != nil {
		t.Fatalf("encodeRedisPublication returned error: %v", err)
	}

	if err := subscriber.handlePayload(context.Background(), payload); err != nil {
		t.Fatalf("handlePayload returned error: %v", err)
	}
	assertSessionQueued(t, session, envelope)
}

func TestRedisSubscriberHandlePayloadRejectsInvalidJSON(t *testing.T) {
	subscriber := NewRedisSubscriber(nil, "admin_go:realtime:test", NoopPublisher{})

	if err := subscriber.handlePayload(context.Background(), []byte(`not-json`)); err == nil {
		t.Fatal("expected invalid redis publication json to fail")
	}
}
