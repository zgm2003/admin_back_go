package canvas

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

func TestRepositoryListAssetsFiltersAndOrders(t *testing.T) {
	repo, mock, closeDB := newCanvasMockRepository(t)
	defer closeDB()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `canvas_assets` WHERE is_del = ? AND status = ? AND type = ? AND ((title LIKE ? OR slug LIKE ? OR description LIKE ?))")).
		WithArgs(IsDelActive, StatusEnabled, AssetTypeImage, "%sky%", "%sky%", "%sky%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `canvas_assets` WHERE is_del = ? AND status = ? AND type = ? AND ((title LIKE ? OR slug LIKE ? OR description LIKE ?)) ORDER BY updated_at DESC, id DESC LIMIT ?")).
		WithArgs(IsDelActive, StatusEnabled, AssetTypeImage, "%sky%", "%sky%", "%sky%", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "type", "category", "title", "cover_url", "description", "content", "url", "tags_json", "status", "is_del", "created_at", "updated_at"}).
			AddRow(int64(2), "sky", AssetTypeImage, "bg", "Sky", "", "blue", "", "https://example.test/sky.png", "[]", StatusEnabled, IsDelActive, now, now))

	rows, total, err := repo.ListAssets(context.Background(), AssetListQuery{CurrentPage: 1, PageSize: 20, Keyword: "sky", Type: AssetTypeImage, Status: StatusEnabled, IsDel: IsDelActive})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Slug != "sky" {
		t.Fatalf("unexpected asset list: rows=%#v total=%d err=%v", rows, total, err)
	}
	assertCanvasMockExpectations(t, mock)
}

func TestRepositorySoftDeleteAssetAndUniqueSlugWriteContracts(t *testing.T) {
	repo, mock, closeDB := newCanvasMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `canvas_assets` SET `is_del`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WithArgs(IsDelDeleted, sqlmock.AnyArg(), int64(7), IsDelActive).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `canvas_assets`")).
		WillReturnError(assertDuplicateSlugError{})
	mock.ExpectRollback()

	if err := repo.SoftDeleteAsset(context.Background(), 7); err != nil {
		t.Fatalf("soft delete asset: %v", err)
	}
	if _, err := repo.CreateAsset(context.Background(), Asset{Slug: "dup", Type: AssetTypeText, Title: "Dup", Status: StatusEnabled, IsDel: IsDelActive}); err == nil {
		t.Fatalf("expected duplicate slug insert error")
	}
	assertCanvasMockExpectations(t, mock)
}

type assertDuplicateSlugError struct{}

func (assertDuplicateSlugError) Error() string { return "Error 1062 duplicate slug" }

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
