package realtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/middleware"
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

	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, infrarealtime.NewSession(nil, infrarealtime.SessionOptions{}), infrarealtime.Envelope{
		Type:      TypePingV1,
		RequestID: "rid-1",
		Data:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if len(replies) != 1 || replies[0].Type != TypePongV1 || replies[0].RequestID != "rid-1" {
		t.Fatalf("unexpected replies: %#v", replies)
	}
}

func TestHandleClientEnvelopeSubscribesOnlyAllowedIdentityTopics(t *testing.T) {
	service := NewService(time.Second)
	session := infrarealtime.NewSession(nil, infrarealtime.SessionOptions{})

	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, session, infrarealtime.Envelope{
		Type:      TypeSubscribeV1,
		RequestID: "rid-subscribe",
		Data:      json.RawMessage(`{"topics":["user:7","session:9","platform:admin"]}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if len(replies) != 1 || replies[0].Type != TypeSubscribedV1 || replies[0].RequestID != "rid-subscribe" {
		t.Fatalf("unexpected subscribe replies: %#v", replies)
	}

	var data map[string][]string
	if err := json.Unmarshal(replies[0].Data, &data); err != nil {
		t.Fatalf("invalid subscribe data: %v", err)
	}
	if got := data["topics"]; !reflect.DeepEqual(got, []string{"user:7", "session:9", "platform:admin"}) {
		t.Fatalf("unexpected subscribed topics: %#v", got)
	}
	if got := session.Topics(); !reflect.DeepEqual(got, []string{"platform:admin", "session:9", "user:7"}) {
		t.Fatalf("subscription state was not applied: %#v", got)
	}

	replies, err = service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, session, infrarealtime.Envelope{
		Type: TypeSubscribeV1, RequestID: "rid-replace", Data: json.RawMessage(`{"topics":["user:7"]}`),
	})
	if err != nil || len(replies) != 1 {
		t.Fatalf("replace subscription replies=%#v err=%v", replies, err)
	}
	if got := session.Topics(); !reflect.DeepEqual(got, []string{"user:7"}) {
		t.Fatalf("subscription did not replace old topics: %#v", got)
	}
}

func TestHandleClientEnvelopeRejectsUnauthorizedTopic(t *testing.T) {
	service := NewService(time.Second)

	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, infrarealtime.NewSession(nil, infrarealtime.SessionOptions{}), infrarealtime.Envelope{
		Type:      TypeSubscribeV1,
		RequestID: "rid-subscribe",
		Data:      json.RawMessage(`{"topics":["user:8"]}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if len(replies) != 1 || replies[0].Type != TypeErrorV1 || replies[0].RequestID != "rid-subscribe" {
		t.Fatalf("expected error reply, got %#v", replies)
	}

	var data map[string]any
	if err := json.Unmarshal(replies[0].Data, &data); err != nil {
		t.Fatalf("invalid error data: %v", err)
	}
	if data["code"] != float64(403) {
		t.Fatalf("expected unauthorized topic code 403, got %#v", data)
	}
}

func TestHandleClientEnvelopeRejectsUnsupportedType(t *testing.T) {
	service := NewService(time.Second)

	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, infrarealtime.NewSession(nil, infrarealtime.SessionOptions{}), infrarealtime.Envelope{
		Type:      "client.unknown.v1",
		RequestID: "rid-1",
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned error: %v", err)
	}
	if len(replies) != 1 || replies[0].Type != TypeErrorV1 {
		t.Fatalf("expected error reply, got %#v", replies)
	}
}

func TestEnvelopeMalformedKnownPayloadIsRejectedWithoutGuessing(t *testing.T) {
	service := NewService(time.Second)
	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, infrarealtime.NewSession(nil, infrarealtime.SessionOptions{}), infrarealtime.Envelope{
		Type: TypeSubscribeV1, RequestID: "rid-malformed", Data: json.RawMessage(`{"topics":["user:7"],"fallback":true}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope returned transport error: %v", err)
	}
	if len(replies) != 1 || replies[0].Type != TypeErrorV1 {
		t.Fatalf("expected closed-contract error reply, got %#v", replies)
	}
}

type fakeResumeRepository struct {
	query  ResumeQuery
	result *ResumeResult
	err    error
}

func (f *fakeResumeRepository) ResumeUser(_ context.Context, query ResumeQuery) (*ResumeResult, error) {
	f.query = query
	return f.result, f.err
}

func TestResumeReturnsPersistedEventsInSequenceOrder(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	firstID, err := infrarealtime.NewEventID(now)
	if err != nil {
		t.Fatalf("first event ID: %v", err)
	}
	secondID, err := infrarealtime.NewEventID(now.Add(time.Second))
	if err != nil {
		t.Fatalf("second event ID: %v", err)
	}
	repository := &fakeResumeRepository{result: &ResumeResult{Events: []Event{
		{Sequence: 10, EventID: firstID, EventType: TypeNotificationCreatedV1, TargetType: TargetTypeUser, TargetID: "7", Durability: string(infrarealtime.Durable), PayloadJSON: `{"task_id":1,"title":"one","content":"body","link":"/","level":"normal","notification_type":"info"}`, OccurredAt: now},
		{Sequence: 11, EventID: secondID, EventType: TypeAIResponseCompletedV1, TargetType: TargetTypeUser, TargetID: "7", Durability: string(infrarealtime.Durable), PayloadJSON: `{"conversation_id":3,"request_id":"request-2","assistant_message_id":9}`, OccurredAt: now.Add(time.Second)},
	}, LatestSequence: 11}}
	service := NewService(time.Second, WithEventReader(repository))
	after := uint64(9)
	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, infrarealtime.NewSession(nil, infrarealtime.SessionOptions{}), infrarealtime.Envelope{
		Type: TypeResumeV1, RequestID: "rid-resume", Data: json.RawMessage(`{"after_sequence":9}`),
	})
	if err != nil {
		t.Fatalf("HandleClientEnvelope resume returned error: %v", err)
	}
	if repository.query.UserID != 7 || repository.query.AfterSequence != after || repository.query.Limit != MaxResumeLimit {
		t.Fatalf("unexpected resume query: %#v", repository.query)
	}
	if len(replies) != 2 || replies[0].Sequence != 10 || replies[1].Sequence != 11 {
		t.Fatalf("unexpected ordered replies: %#v", replies)
	}
}

func TestResumeReturnsConfirmedResyncRequiredPayload(t *testing.T) {
	repository := &fakeResumeRepository{result: &ResumeResult{ResyncRequired: true, LatestSequence: 123}}
	service := NewService(time.Second, WithEventReader(repository))
	service.now = func() time.Time { return time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC) }

	replies, err := service.HandleClientEnvelope(context.Background(), &middleware.AuthIdentity{
		UserID: 7, SessionID: 9, Platform: "admin",
	}, infrarealtime.NewSession(nil, infrarealtime.SessionOptions{}), infrarealtime.Envelope{
		Type: TypeResumeV1, RequestID: "resume-1", Data: json.RawMessage(`{"after_sequence":4}`),
	})
	if err != nil {
		t.Fatalf("resume returned error: %v", err)
	}
	if len(replies) != 1 || replies[0].Type != TypeResyncRequiredV1 || replies[0].Sequence != 0 || replies[0].Durability != infrarealtime.Ephemeral {
		t.Fatalf("unexpected resync replies: %#v", replies)
	}
	if string(replies[0].Data) != `{"latest_sequence":123}` {
		t.Fatalf("unexpected resync payload: %s", replies[0].Data)
	}
}
