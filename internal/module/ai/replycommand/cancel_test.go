package replycommand

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeCancelSubscription struct {
	signal chan struct{}
}

func TestRequestCancelBuildsStoppedMessageFromServerPrefix(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	requestedAt := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "user_id", "conversation_id", "state", "cancel_requested_at", "delivery_seq"}).
			AddRow(41, "request-1", 7, 3, StateRunning, nil, 4))
	mock.ExpectQuery("SELECT .* FROM `ai_conversations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "agent_id", "title", "is_del"}).AddRow(3, 7, 5, "", 2))
	mock.ExpectQuery("SELECT .*delivery_seq.*delta.* FROM `ai_reply_delivery_chunks`").
		WillReturnRows(sqlmock.NewRows([]string{"delivery_seq", "delta"}).
			AddRow(1, "1").AddRow(2, "2").AddRow(3, "3").AddRow(4, "4"))
	mock.ExpectExec("INSERT INTO `ai_messages`").WillReturnResult(sqlmock.NewResult(97, 1))
	mock.ExpectExec("UPDATE `ai_conversations` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("DELETE FROM ai_reply_delivery_chunks").WillReturnResult(sqlmock.NewResult(0, 4))

	result, err := repository.RequestCancel(context.Background(), RequestCancelInput{
		ConversationID: 3,
		UserID:         7,
		RequestID:      "request-1",
		DeliveredSeq:   4,
		Now:            requestedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != CancelStatusStopped || result.CommandID != 41 || result.AssistantMessageID != 97 || !result.SettlementPending {
		t.Fatalf("result=%+v", result)
	}
	if !result.DeliveryConsistent || result.StopDeliverySeq != 4 {
		t.Fatalf("delivery result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestCancelReturnsTerminalWinnerWithoutChangingFacts(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	assistantID := int64(97)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "user_id", "conversation_id", "state", "assistant_message_id"}).
			AddRow(41, "request-1", 7, 3, StateSucceeded, assistantID))
	mock.ExpectCommit()

	result, err := repository.RequestCancel(context.Background(), RequestCancelInput{
		ConversationID: 3,
		UserID:         7,
		RequestID:      "request-1",
		DeliveredSeq:   2,
		Now:            time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != CancelStatusAlreadyTerminal || result.AssistantMessageID != assistantID || result.SettlementPending {
		t.Fatalf("result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeCancelSubscription) Signal() <-chan struct{} { return f.signal }
func (f *fakeCancelSubscription) Close() error            { return nil }

func TestRedisCancelSignalUsesCommandScopedChannel(t *testing.T) {
	var publishedChannel string
	publisher := &RedisCancelPublisher{publish: func(_ context.Context, channel string) error {
		publishedChannel = channel
		return errors.New("redis unavailable")
	}}
	if err := publisher.PublishCancel(context.Background(), 41); err == nil || publishedChannel != "ai:reply:cancel:41" {
		t.Fatalf("channel=%q err=%v", publishedChannel, err)
	}

	want := &fakeCancelSubscription{signal: make(chan struct{})}
	var subscribedChannel string
	subscriber := &RedisCancelSubscriber{subscribe: func(_ context.Context, channel string) (CancelSubscription, error) {
		subscribedChannel = channel
		return want, nil
	}}
	got, err := subscriber.SubscribeCancel(context.Background(), 41)
	if err != nil || got != want || subscribedChannel != "ai:reply:cancel:41" {
		t.Fatalf("subscription=%T channel=%q err=%v", got, subscribedChannel, err)
	}
}
