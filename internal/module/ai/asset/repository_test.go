package asset

import (
	"context"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryListFiltersAndOrdersAIAssets(t *testing.T) {
	repo, mock, closeDB := newAssetMockRepository(t)
	defer closeDB()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `ai_assets` WHERE is_del = ? AND status = ? AND type = ? AND ((title LIKE ? OR slug LIKE ? OR description LIKE ?))")).
		WithArgs(IsDelActive, StatusEnabled, AssetTypeImage, "%sky%", "%sky%", "%sky%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_assets` WHERE is_del = ? AND status = ? AND type = ? AND ((title LIKE ? OR slug LIKE ? OR description LIKE ?)) ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?")).
		WithArgs(IsDelActive, StatusEnabled, AssetTypeImage, "%sky%", "%sky%", "%sky%", 5, 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "type", "category", "title", "cover_url", "description", "content", "url", "tags_json", "status", "is_del", "created_at", "updated_at"}).
			AddRow(int64(2), "sky", AssetTypeImage, "bg", "Sky", "", "blue", "", "https://example.test/sky.png", "[]", StatusEnabled, IsDelActive, now, now))

	rows, total, err := repo.List(context.Background(), ListQuery{CurrentPage: 2, PageSize: 5, Keyword: "sky", Type: AssetTypeImage, Status: StatusEnabled, IsDel: IsDelActive})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Slug != "sky" {
		t.Fatalf("unexpected asset list: rows=%#v total=%d err=%v", rows, total, err)
	}
	assertAssetMockExpectations(t, mock)
}

func TestRepositoryCreateUpdateAndSoftDeleteWriteAIAssets(t *testing.T) {
	repo, mock, closeDB := newAssetMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_assets`")).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `ai_assets` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `ai_assets` SET `is_del`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WithArgs(IsDelDeleted, sqlmock.AnyArg(), int64(7), IsDelActive).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	id, err := repo.Create(context.Background(), Asset{Slug: "clip", Type: AssetTypeVideo, Title: "Clip"})
	if err != nil || id != 9 {
		t.Fatalf("unexpected create id=%d err=%v", id, err)
	}
	if err := repo.Update(context.Background(), 9, Asset{Slug: "clip-2", Type: AssetTypeVideo, Title: "Clip 2", Status: StatusEnabled, IsDel: IsDelActive}); err != nil {
		t.Fatalf("update asset: %v", err)
	}
	if err := repo.SoftDelete(context.Background(), 7); err != nil {
		t.Fatalf("soft delete asset: %v", err)
	}
	assertAssetMockExpectations(t, mock)
}

func newAssetMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
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

func assertAssetMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

var _ = database.Client{}
