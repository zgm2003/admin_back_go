package contextengine

import (
	"context"
	"testing"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestToolGroupRequiresPairedResult(t *testing.T) {
	callID := "call-1"
	if _, complete := turnToolGroups([]conversationToolRow{{
		ID: 1, RunID: 2, CallID: &callID, ToolCode: "lookup", Status: "success", ArgumentsJSON: `{}`, ResultJSON: nil,
	}}); complete {
		t.Fatal("missing tool result was accepted")
	}
	result := `{"ok":true}`
	groups, complete := turnToolGroups([]conversationToolRow{{
		ID: 1, RunID: 2, CallID: &callID, ToolCode: "lookup", Status: "success", ArgumentsJSON: `{}`, ResultJSON: &result,
	}})
	if !complete || len(groups) != 1 || groups[0].CallID != callID {
		t.Fatalf("complete group=%+v complete=%v", groups, complete)
	}
}

func TestConversationTurnPageUsesStableDescendingAnchorsAndFixedQueries(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewConversationRepositoryWithDB(db)
	mock.ExpectQuery(`SELECT m\.id FROM ai_messages m .* ORDER BY m\.id DESC LIMIT \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uint64(30)).AddRow(uint64(20)).AddRow(uint64(10)))
	mock.ExpectQuery(`SELECT conversation\.id AS conversation_id,.*FROM ai_messages m .*m\.id IN \(\?,\?\)`).
		WillReturnRows(conversationTurnRows().
			AddRow(uint64(7), uint64(5), uint64(9), uint64(30), enum.AIMessageRoleUser, "new", nil, uint64(31), "succeeded", uint64(32), enum.AIRunStatusSuccess, uint64(5), uint64(9), uint64(33), enum.AIMessageRoleAssistant, "new answer", "completed").
			AddRow(uint64(7), uint64(5), uint64(9), uint64(20), enum.AIMessageRoleUser, "old", nil, uint64(21), "succeeded", uint64(22), enum.AIRunStatusSuccess, uint64(5), uint64(9), uint64(23), enum.AIMessageRoleAssistant, "old answer", "completed"))
	mock.ExpectQuery("SELECT id, run_id, call_id, tool_code, status, arguments_json, result_json FROM `ai_tool_calls` .*run_id IN \\(\\?,\\?\\).*ORDER BY run_id ASC, id ASC").
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "call_id", "tool_code", "status", "arguments_json", "result_json"}))

	page, err := repository.PageCompleteBefore(context.Background(), 7, 5, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 2 || page.Turns[0].UserMessage.ID != 30 || page.Turns[1].UserMessage.ID != 20 ||
		page.NextBeforeUserMessageID == nil || *page.NextBeforeUserMessageID != 20 {
		t.Fatalf("page=%+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func conversationTurnRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"conversation_id", "conversation_user_id", "conversation_agent_id", "user_message_id", "user_role", "user_content", "user_meta_json",
		"command_id", "command_state", "run_id", "run_status", "run_user_id", "run_agent_id",
		"assistant_message_id", "assistant_role", "assistant_content", "assistant_delivery",
	})
}
