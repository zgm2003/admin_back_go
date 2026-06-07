package canvas

import (
	"context"
	"testing"

	"admin_back_go/internal/infra/database"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryListAgentsBySceneSkipsBlankScene(t *testing.T) {
	repo, mock, closeDB := newCanvasMockRepository(t)
	defer closeDB()

	rows, err := repo.ListAgentsByScene(context.Background(), "  ")
	if err != nil || len(rows) != 0 {
		t.Fatalf("blank scene must return an empty list without SQL, rows=%#v err=%v", rows, err)
	}
	assertCanvasMockExpectations(t, mock)
}

func TestRepositoryNilClientIsNotConfigured(t *testing.T) {
	if repo := NewGormRepository(nil); repo != nil {
		t.Fatalf("nil database client must not create repository: %#v", repo)
	}
	if _, err := (&GormRepository{}).ListAgentsByScene(context.Background(), "canvas_text_generate"); err != ErrRepositoryNotConfigured {
		t.Fatalf("nil gorm repository must return ErrRepositoryNotConfigured, got %v", err)
	}
}

func newCanvasMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: false, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return &GormRepository{db: db}, mock, func() { _ = sqlDB.Close() }
}

func assertCanvasMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

var _ = database.Client{}
