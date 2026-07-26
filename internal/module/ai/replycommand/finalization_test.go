package replycommand

import (
	"context"
	"testing"
	"time"

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
	mock.ExpectExec("INSERT INTO `ai_messages`").WillReturnResult(sqlmock.NewResult(22, 1))
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
