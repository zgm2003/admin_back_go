package prompt

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryListFiltersAndOrdersAIPrompts(t *testing.T) {
	repo, mock, closeDB := newPromptMockRepository(t)
	defer closeDB()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `ai_prompts` WHERE is_del = ? AND status = ? AND category = ? AND ((title LIKE ? OR slug LIKE ? OR prompt LIKE ?)) AND JSON_CONTAINS(CAST(tags_json AS JSON), JSON_QUOTE(?))")).
		WithArgs(IsDelActive, StatusEnabled, "style", "%cat%", "%cat%", "%cat%", "poster").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_prompts` WHERE is_del = ? AND status = ? AND category = ? AND ((title LIKE ? OR slug LIKE ? OR prompt LIKE ?)) AND JSON_CONTAINS(CAST(tags_json AS JSON), JSON_QUOTE(?)) ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?")).
		WithArgs(IsDelActive, StatusEnabled, "style", "%cat%", "%cat%", "%cat%", "poster", 5, 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "category", "title", "cover_url", "prompt", "preview", "tags_json", "source_url", "status", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), "cat", "style", "Cat", "", "draw cat", "", "[]", "", StatusEnabled, IsDelActive, now, now))

	rows, total, err := repo.List(context.Background(), ListQuery{CurrentPage: 2, PageSize: 5, Keyword: "cat", Category: "style", Tags: []string{"poster"}, Status: StatusEnabled, IsDel: IsDelActive})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Slug != "cat" {
		t.Fatalf("unexpected prompt list rows=%#v total=%d err=%v", rows, total, err)
	}
	assertPromptMockExpectations(t, mock)
}

func TestRepositoryCreateWritesAIPrompts(t *testing.T) {
	repo, mock, closeDB := newPromptMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_prompts`")).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()

	id, err := repo.Create(context.Background(), Prompt{Slug: "cat", Title: "Cat", Prompt: "draw cat"})
	if err != nil || id != 9 {
		t.Fatalf("unexpected create id=%d err=%v", id, err)
	}
	assertPromptMockExpectations(t, mock)
}

func TestRepositorySoftDeleteBatchRequiresEveryPromptIDToMatch(t *testing.T) {
	repo, mock, closeDB := newPromptMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `ai_prompts` SET `is_del`=?,`updated_at`=? WHERE id IN (?,?) AND is_del = ?")).
		WithArgs(IsDelDeleted, sqlmock.AnyArg(), int64(3), int64(4), IsDelActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.SoftDeleteBatch(context.Background(), []int64{3, 4})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound when only part of batch matched, got %v", err)
	}
	assertPromptMockExpectations(t, mock)
}

func newPromptMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
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

func assertPromptMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

var _ = database.Client{}
