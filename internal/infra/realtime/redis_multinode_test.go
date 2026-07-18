package realtime

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestMultiNodeRedisFanoutDeliversOncePerSubscribedAPIInstance(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_REDIS_ADDR is required for realtime multi-node test")
	}
	client := redis.NewClient(&redis.Options{Addr: address, DB: 15})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping test Redis: %v", err)
	}
	channel := "admin_go:realtime:test:" + time.Now().Format("150405.000000000")

	managerA, sessionA := subscribedTestNode(7)
	managerB, sessionB := subscribedTestNode(7)
	defer managerA.CloseAll()
	defer managerB.CloseAll()
	subscriberA := NewRedisSubscriber(client, channel, NewLocalPublisher(managerA))
	subscriberB := NewRedisSubscriber(client, channel, NewLocalPublisher(managerB))
	if err := subscriberA.Start(ctx); err != nil {
		t.Fatalf("start subscriber A: %v", err)
	}
	if err := subscriberB.Start(ctx); err != nil {
		t.Fatalf("start subscriber B: %v", err)
	}
	defer subscriberA.Shutdown(context.Background())
	defer subscriberB.Shutdown(context.Background())

	publisher := NewRedisPublisher(client, channel)
	first := mustRealtimeEnvelope(t, "notification.created.v1", "rid-multi-one")
	if err := publisher.Publish(ctx, Publication{Platform: "admin", UserID: 7, Envelope: first}); err != nil {
		t.Fatalf("publish first event: %v", err)
	}
	assertSessionQueuedEventually(t, sessionA, first)
	assertSessionQueuedEventually(t, sessionB, first)
	assertSessionEmpty(t, sessionA)
	assertSessionEmpty(t, sessionB)

	if err := subscriberB.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown subscriber B: %v", err)
	}
	second := mustRealtimeEnvelope(t, "notification.created.v1", "rid-multi-two")
	if err := publisher.Publish(ctx, Publication{Platform: "admin", UserID: 7, Envelope: second}); err != nil {
		t.Fatalf("publish with one API node offline: %v", err)
	}
	assertSessionQueuedEventually(t, sessionA, second)
	time.Sleep(50 * time.Millisecond)
	assertSessionEmpty(t, sessionB)
}

func subscribedTestNode(userID int64) (*Manager, *Session) {
	manager := NewManager()
	session := NewSession(nil, SessionOptions{SendBuffer: 4})
	session.ReplaceTopics([]string{fmt.Sprintf("user:%d", userID)})
	manager.Register(fmt.Sprintf("admin:%d:1", userID), session)
	return manager, session
}

func assertSessionQueuedEventually(t *testing.T, session *Session, envelope Envelope) {
	t.Helper()
	select {
	case got := <-session.send:
		if got.EventID != envelope.EventID {
			t.Fatalf("unexpected event: got=%#v want=%#v", got, envelope)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("event %s was not delivered", envelope.EventID)
	}
}
