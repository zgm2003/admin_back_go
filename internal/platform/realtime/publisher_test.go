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

func TestManagerSendToUserPublishesToEveryMatchingSession(t *testing.T) {
	manager := NewManager()
	first := NewSession(nil, SessionOptions{SendBuffer: 1})
	second := NewSession(nil, SessionOptions{SendBuffer: 1})
	otherUser := NewSession(nil, SessionOptions{SendBuffer: 1})
	otherPlatform := NewSession(nil, SessionOptions{SendBuffer: 1})
	defer first.Close()
	defer second.Close()
	defer otherUser.Close()
	defer otherPlatform.Close()
	manager.Register("admin:7:1", first)
	manager.Register("admin:7:2", second)
	manager.Register("admin:8:1", otherUser)
	manager.Register("app:7:1", otherPlatform)

	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid-user")
	if err := manager.SendToUser("admin", 7, envelope); err != nil {
		t.Fatalf("SendToUser returned error: %v", err)
	}

	assertSessionQueued(t, first, envelope)
	assertSessionQueued(t, second, envelope)
	assertSessionEmpty(t, otherUser)
	assertSessionEmpty(t, otherPlatform)
}

func TestLocalPublisherPublishesToUserTarget(t *testing.T) {
	manager := NewManager()
	session := NewSession(nil, SessionOptions{SendBuffer: 1})
	defer session.Close()
	manager.Register("admin:7:9", session)

	publisher := NewLocalPublisher(manager)
	envelope := mustRealtimeEnvelope(t, "notification.created.v1", "rid-user")
	if err := publisher.Publish(context.Background(), Publication{
		Platform: "admin",
		UserID:   7,
		Envelope: envelope,
	}); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	assertSessionQueued(t, session, envelope)
}

func TestLocalPublisherRequiresDeliveryTarget(t *testing.T) {
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

func assertSessionQueued(t *testing.T, session *Session, envelope Envelope) {
	t.Helper()
	select {
	case got := <-session.send:
		if got.Type != envelope.Type || got.RequestID != envelope.RequestID {
			t.Fatalf("unexpected envelope: got=%#v want=%#v", got, envelope)
		}
	default:
		t.Fatal("expected envelope in session send queue")
	}
}

func assertSessionEmpty(t *testing.T, session *Session) {
	t.Helper()
	select {
	case got := <-session.send:
		t.Fatalf("expected no envelope, got %#v", got)
	default:
	}
}
