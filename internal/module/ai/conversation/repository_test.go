package aiconversation

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryListFiltersByLastMessageTimeAndIDTuple(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

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
