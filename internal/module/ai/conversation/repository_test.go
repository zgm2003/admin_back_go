package aiconversation

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestConversationModelMapsLastReadMessageID(t *testing.T) {
	parsed, err := schema.Parse(&Conversation{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Conversation schema: %v", err)
	}
	field := parsed.LookUpField("LastReadMessageID")
	if field == nil {
		t.Fatal("Conversation must persist LastReadMessageID")
	}
	if field.DBName != "last_read_message_id" {
		t.Fatalf("LastReadMessageID column mismatch: %q", field.DBName)
	}
	if field.FieldType != reflect.TypeOf(int64(0)) {
		t.Fatalf("LastReadMessageID type mismatch: %v", field.FieldType)
	}
	if parsed.LookUpField("UnreadCount") != nil {
		t.Fatal("derived UnreadCount must not be part of the Conversation persistence model")
	}
}

func TestConversationUnreadCountDTOIsNonNegative(t *testing.T) {
	for _, value := range []any{ConversationItem{}, ReadCursorResponse{}} {
		field, ok := reflect.TypeOf(value).FieldByName("UnreadCount")
		if !ok {
			t.Fatalf("%T must expose UnreadCount", value)
		}
		if field.Type.Kind() != reflect.Uint64 {
			t.Fatalf("%T UnreadCount must be non-negative, type=%v", value, field.Type)
		}
	}
}

func TestRepositoryUnreadCountsGroupsTwoConversationsAndFiltersVisibleAssistantMessages(t *testing.T) {
	sqlDB, mock, db := newConversationRepositoryTestDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })

	mock.ExpectQuery("(?s)SELECT m\\.conversation_id, COUNT\\(\\*\\) AS unread_count FROM ai_messages m JOIN ai_conversations c ON c\\.id = m\\.conversation_id WHERE m\\.conversation_id IN \\(\\?,\\?\\) AND m\\.role = \\? AND m\\.is_del = \\? AND m\\.id > c\\.last_read_message_id AND c\\.is_del = \\? GROUP BY .*m.*conversation_id").
		WithArgs(int64(8), int64(6), 2, 2, 2).
		WillReturnRows(sqlmock.NewRows([]string{"conversation_id", "unread_count"}).AddRow(8, 3).AddRow(6, 1))

	counts, err := (&GormRepository{db: db}).UnreadCounts(context.Background(), []int64{8, 6})
	if err != nil {
		t.Fatalf("UnreadCounts returned error: %v", err)
	}
	if len(counts) != 2 || counts[8] != 3 || counts[6] != 1 {
		t.Fatalf("unexpected unread counts: %#v", counts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unread counts must use exactly one grouped query: %v", err)
	}
}

func TestRepositoryReadCursorUsesVisibleAssistantAndGreatest(t *testing.T) {
	sqlDB, mock, db := newConversationRepositoryTestDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("(?s)SELECT .*last_read_message_id.* FROM .*ai_conversations.* WHERE id = \\? AND user_id = \\? AND is_del = \\?.*LIMIT \\? FOR UPDATE").
		WithArgs(int64(3), int64(7), 2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "last_read_message_id"}).AddRow(3, 4))
	mock.ExpectQuery("(?s)SELECT .*id.* FROM .*ai_messages.* WHERE id = \\? AND conversation_id = \\? AND role = \\? AND is_del = \\?.*LIMIT \\? FOR UPDATE").
		WithArgs(int64(9), int64(3), 2, 2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectExec("UPDATE .*ai_conversations.* SET .*last_read_message_id.*GREATEST\\(last_read_message_id, \\?\\).* WHERE id = \\? AND user_id = \\? AND is_del = \\?").
		WithArgs(int64(9), int64(3), int64(7), 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	cursor, valid, err := (&GormRepository{db: db}).AdvanceReadCursor(context.Background(), 3, 7, 9)
	if err != nil {
		t.Fatalf("AdvanceReadCursor returned error: %v", err)
	}
	if !valid || cursor != 9 {
		t.Fatalf("unexpected cursor result: cursor=%d valid=%v", cursor, valid)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("cursor update did not validate and advance atomically: %v", err)
	}
}

func TestRepositoryListFiltersByLastMessageTimeAndIDTuple(t *testing.T) {
	sqlDB, mock, db := newConversationRepositoryTestDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })

	mock.ExpectQuery(`(?s)SELECT .* FROM ai_conversations c .*WHERE c\.user_id = \? AND c\.is_del = \? AND \(\(c\.last_message_at < \? OR \(c\.last_message_at = \? AND c\.id < \?\)\)\).*ORDER BY c\.last_message_at DESC,c\.id DESC LIMIT \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "agent_id", "title", "last_message_at", "is_del", "created_at", "updated_at", "agent_name"}))

	cursorTime := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	if _, _, err := (&GormRepository{db: db}).List(context.Background(), ListQuery{UserID: 7, BeforeTime: &cursorTime, BeforeID: 20, Limit: 20}); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("tuple cursor query was not executed: %v", err)
	}
}

func newConversationRepositoryTestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *gorm.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open: %v", err)
	}
	return sqlDB, mock, db
}
