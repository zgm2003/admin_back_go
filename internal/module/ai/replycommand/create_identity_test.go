package replycommand

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateReplyPersistsRunBeforeCommandWithDirectIdentity(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	repository.now = func() time.Time { return time.Date(2026, 8, 4, 15, 0, 0, 0, time.UTC) }
	input := testCreateReplyInput(3, 7, "direct-run-identity", "hello")

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `ai_conversations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "agent_id", "is_del"}).AddRow(3, 7, 1, enum.CommonNo))
	mock.ExpectExec("INSERT INTO `ai_messages`").WillReturnResult(sqlmock.NewResult(51, 1))
	mock.ExpectExec("INSERT INTO `ai_runs`").WillReturnResult(sqlmock.NewResult(61, 1))
	mock.ExpectExec("INSERT INTO `ai_reply_commands`.*`run_id`").WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectExec("INSERT INTO `ai_run_events`").WillReturnResult(sqlmock.NewResult(71, 1))
	mock.ExpectExec("INSERT INTO `ai_usage_charges`").WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectExec("UPDATE `ai_conversations` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `ai_conversations` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repository.CreateReply(context.Background(), input)
	if err != nil || result.UserMessageID != 51 || result.RunID != 61 || result.CommandID != 41 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
