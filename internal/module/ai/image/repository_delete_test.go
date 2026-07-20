package aiimage

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDeleteTaskSoftDeletesTaskAndFilesInOneTransaction(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ` + "`ai_image_tasks`" + ` SET .*` + "`is_del`" + `=\?.* WHERE .*platform = \? AND user_id = \? AND id = \?.*is_del = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE ` + "`ai_image_files`" + ` SET ` + "`is_del`" + `=\? WHERE task_id = \? AND is_del = \?`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	if err := (&GormRepository{db: db}).DeleteTask(context.Background(), 7, 11, "admin"); err != nil {
		t.Fatalf("DeleteTask returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("delete was not transactional soft-delete: %v", err)
	}
}
