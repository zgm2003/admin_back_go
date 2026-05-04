package realtime

import (
	"context"
	"errors"
	"testing"
)

func TestLocalPublisherPublishesToLocalManager(t *testing.T) {
	manager := NewManager()
	session := NewSession(nil, SessionOptions{SendBuffer: 1})
	defer session.Close()
	manager.Register("admin:7:9", session)

	publisher := NewLocalPublisher(manager)
	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid-1")
	if err := publisher.Publish(context.Background(), Publication{
		SessionKey: "admin:7:9",
		Envelope:   envelope,
	}); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	select {
	case got := <-session.send:
		if got.Type != envelope.Type || got.RequestID != envelope.RequestID {
			t.Fatalf("unexpected published envelope: %#v", got)
		}
	default:
		t.Fatal("expected envelope in session send queue")
	}
}

func TestLocalPublisherRequiresSessionKey(t *testing.T) {
	publisher := NewLocalPublisher(NewManager())
	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid-1")

	err := publisher.Publish(context.Background(), Publication{Envelope: envelope})
	if !errors.Is(err, ErrPublicationTargetRequired) {
		t.Fatalf("expected ErrPublicationTargetRequired, got %v", err)
	}
}

func TestNoopPublisherIgnoresPublicationWithoutSideEffects(t *testing.T) {
	publisher := NoopPublisher{}
	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid-1")

	if err := publisher.Publish(context.Background(), Publication{
		SessionKey: "admin:missing",
		Envelope:   envelope,
	}); err != nil {
		t.Fatalf("Noop Publish returned error: %v", err)
	}
}
