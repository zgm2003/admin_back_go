package replycommand

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestFinalizePaidCommandInTransactionPublishesAssistantAndClosesLease(t *testing.T) {
	repository, db, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "user_id", "conversation_id", "state"}).
			AddRow(41, "request-1", 7, 3, StateRunning))
	mock.ExpectQuery("SELECT .* FROM `ai_messages`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO `ai_messages`").
		WithArgs(int64(3), uint64(41), enum.AIMessageRoleAssistant, "text", "hello", nil, DeliveryStateCompleted, enum.CommonNo, now, now).
		WillReturnResult(sqlmock.NewResult(22, 1))
	mock.ExpectExec("UPDATE `ai_conversations`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `ai_reply_commands`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	tx := db.Begin()
	result, err := repository.FinalizePaidCommandInTransaction(context.Background(), tx, PaidCommandFinalizationInput{
		CommandID: 41, UserID: 7, RequestID: "request-1", State: StateSucceeded, Content: "hello", Now: now,
	})
	if err != nil || result == nil || result.AssistantMessageID != 22 {
		t.Fatalf("finalization result=%+v err=%v", result, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledFinalizationReusesStoppedAssistantMessage(t *testing.T) {
	repository, db, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	requestedAt := now.Add(-2 * time.Second)
	createdAt := now.Add(-time.Second)
	stopSeq := uint32(4)
	assistantID := int64(97)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "conversation_id", "state", "cancel_requested_at", "stop_delivery_seq", "assistant_message_id",
		}).AddRow(41, "request-1", 7, 3, StateRunning, requestedAt, stopSeq, assistantID))
	mock.ExpectQuery("SELECT .* FROM `ai_messages`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "conversation_id", "reply_command_id", "role", "content_type", "content", "delivery_state", "is_del", "created_at", "updated_at",
		}).AddRow(assistantID, 3, 41, enum.AIMessageRoleAssistant, "text", "1234", DeliveryStateStopped, enum.CommonNo, createdAt, createdAt))
	mock.ExpectExec("UPDATE `ai_reply_commands`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	tx := db.Begin()
	result, err := repository.FinalizePaidCommandInTransaction(context.Background(), tx, PaidCommandFinalizationInput{
		CommandID: 41, UserID: 7, RequestID: "request-1", State: StateCanceled, Now: now,
	})
	if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err != nil || result == nil || result.AssistantMessageID != assistantID {
		t.Fatalf("finalization result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizePaidCommandFinalizationRejectsInvalidTerminalPayload(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	for _, input := range []PaidCommandFinalizationInput{
		{CommandID: 41, UserID: 7, RequestID: "request-1", State: StateSucceeded, Now: now},
		{CommandID: 41, UserID: 7, RequestID: "request-1", State: StateFailed, ErrorCode: "", ErrorMessage: "failed", Now: now},
		{CommandID: 41, UserID: 7, RequestID: "request-1", State: StateOutcomeUnknown, ErrorCode: "unknown", ErrorMessage: "", Now: now},
		{CommandID: 41, UserID: 7, RequestID: "request-1", State: StatePending, Content: "hello", Now: now},
	} {
		if err := normalizePaidCommandFinalization(&input); err == nil {
			t.Fatalf("invalid terminal payload accepted: %+v", input)
		}
	}

	valid := PaidCommandFinalizationInput{CommandID: 41, UserID: 7, RequestID: "request-1", State: StateCanceled, Now: now}
	if err := normalizePaidCommandFinalization(&valid); err != nil || valid.Content != "" || valid.ErrorCode != "" || valid.ErrorMessage != "" {
		t.Fatalf("valid cancellation normalized incorrectly: %+v err=%v", valid, err)
	}
}

func TestPaidCommandFinalizationSourceStatesAllowFailedRepairOnly(t *testing.T) {
	states, err := paidCommandFinalizationSourceStates(StateFailed, StateFailed)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, state := range states {
		found = found || state == StateFailed
	}
	if !found {
		t.Fatalf("failed repair source missing from %v", states)
	}
	if _, err := paidCommandFinalizationSourceStates(StateFailed, StateSucceeded); !errors.Is(err, ErrPaidCommandFinalizationConflict) {
		t.Fatalf("failed command changed terminal meaning: %v", err)
	}
}
