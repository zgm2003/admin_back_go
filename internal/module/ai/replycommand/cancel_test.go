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

func TestRequestCancelPersistsFirstIntentWithoutTerminalizing(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	requestedAt := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

	expectCancelLookup := func(cancelAt any) {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
			WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "user_id", "conversation_id", "state", "cancel_requested_at"}).AddRow(41, "request-1", 7, 3, StatePending, cancelAt))
		mock.ExpectQuery("SELECT .* FROM `ai_conversations`").
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "agent_id", "title", "is_del"}).AddRow(3, 7, 5, "", 2))
	}

	expectCancelLookup(nil)
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	first, err := repository.RequestCancel(context.Background(), 3, 7, "request-1", requestedAt)
	if err != nil || first == nil || first.State != StatePending || first.CancelRequestedAt == nil || !first.CancelRequestedAt.Equal(requestedAt) || first.FinishedAt != nil {
		t.Fatalf("first cancel=%+v err=%v", first, err)
	}

	expectCancelLookup(requestedAt)
	mock.ExpectCommit()
	repeated, err := repository.RequestCancel(context.Background(), 3, 7, "request-1", requestedAt.Add(time.Minute))
	if err != nil || repeated == nil || repeated.CancelRequestedAt == nil || !repeated.CancelRequestedAt.Equal(requestedAt) {
		t.Fatalf("repeated cancel=%+v err=%v", repeated, err)
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
