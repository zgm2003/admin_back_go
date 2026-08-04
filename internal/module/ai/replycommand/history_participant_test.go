package replycommand

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/requestidentity"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHistoryParticipantRequiresCallerTransaction(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	participant := NewHistoryParticipant(repository)

	if _, err := participant.ReplayInTransaction(context.Background(), root, testHistoryRequest("chat.revision", "changed")); !errors.Is(err, ErrHistoryTransactionRequired) {
		t.Fatalf("root transaction error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryParticipantReplaysEqualTypedIdentityAndRejectsChangedOperation(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	participant := NewHistoryParticipant(repository)
	equal := testHistoryRequest("chat.revision", "changed")
	fingerprint, err := requestidentity.BuildFingerprint(equal.Identity)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	tx := root.Begin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*LIMIT \\?$").
		WithArgs(int64(7), "history-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "request_fingerprint", "request_identity_status", "user_message_id", "state"}).
			AddRow(81, "history-1", fingerprint[:], requestidentity.IdentityStatusReplayable, 71, StatePending))
	mock.ExpectQuery("SELECT .* FROM `ai_runs`.*LIMIT \\?$").WithArgs(int64(7), "history-1", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(91))
	mock.ExpectQuery("SELECT .* FROM `ai_usage_charges`.*LIMIT \\?$").WithArgs(int64(91), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101))

	result, err := participant.ReplayInTransaction(context.Background(), tx, equal)
	if err != nil || result == nil || result.CommandID != 81 || result.RunID != 91 || result.ChargeID != 101 {
		t.Fatalf("replay=%+v err=%v", result, err)
	}
	mock.ExpectRollback()
	_ = tx.Rollback()

	changedOperation := equal
	changedOperation.Identity.Operation = "chat.regeneration"
	mock.ExpectBegin()
	tx = root.Begin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*LIMIT \\?$").
		WithArgs(int64(7), "history-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "request_fingerprint", "request_identity_status"}).
			AddRow(81, "history-1", fingerprint[:], requestidentity.IdentityStatusReplayable))
	if _, err := participant.ReplayInTransaction(context.Background(), tx, changedOperation); !errors.Is(err, requestidentity.ErrRequestIdentityConflict) {
		t.Fatalf("changed operation error=%v", err)
	}
	mock.ExpectRollback()
	_ = tx.Rollback()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryParticipantCreatesExactlyOnePaidReplyIdentityInCallerTransaction(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	participant := NewHistoryParticipant(repository)
	request := testHistoryRequest("chat.revision", "changed")
	base := testCreateReplyInput(3, 7, request.RequestID, "changed")
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	input := HistoryCreateInput{
		HistoryRequest: request, ConversationID: 3, AgentID: 5, ProviderID: 9,
		ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT-4.1 mini", Content: "changed",
		InputSnapshot: "changed", PricingSnapshotJSON: strings.ReplaceAll(base.PricingSnapshotJSON, "test-model", "gpt-4.1-mini"),
		EffectiveMaxTokens: 4096, AcceptedAt: now,
	}

	mock.ExpectBegin()
	tx := root.Begin()
	mock.ExpectExec("INSERT INTO `ai_messages`").WillReturnResult(sqlmock.NewResult(71, 1))
	mock.ExpectExec("INSERT INTO `ai_runs`").WillReturnResult(sqlmock.NewResult(91, 1))
	mock.ExpectExec("INSERT INTO `ai_reply_commands`.*`run_id`").WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectExec("INSERT INTO `ai_run_events`").WillReturnResult(sqlmock.NewResult(92, 1))
	mock.ExpectExec("INSERT INTO `ai_usage_charges`").WillReturnResult(sqlmock.NewResult(101, 1))

	result, err := participant.CreateInTransaction(context.Background(), tx, input)
	if err != nil || result.UserMessageID != 71 || result.CommandID != 81 || result.RunID != 91 || result.ChargeID != 101 {
		t.Fatalf("created=%+v err=%v", result, err)
	}
	mock.ExpectRollback()
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func testHistoryRequest(operation string, text string) HistoryRequest {
	return HistoryRequest{
		UserID:    7,
		RequestID: "history-1",
		Identity: requestidentity.Input{
			UserID: 7, Operation: operation, Modality: "chat", AgentID: 5, ModelID: "gpt-4.1-mini",
			NormalizedText: text, ConversationID: 3, SourceMessageID: 41,
			Options: requestidentity.GenerationOptions{MaxOutputTokens: 4096},
		},
	}
}
