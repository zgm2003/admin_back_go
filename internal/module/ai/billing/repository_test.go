package aibilling

import (
	"context"
	"reflect"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryDeleteSoftDeletesRule(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `ai_billing_rules` SET `is_del`=?,`updated_at`=? WHERE is_del = ? AND id = ?")).
		WithArgs(enum.CommonYes, sqlmock.AnyArg(), enum.CommonNo, uint64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete error=%v", err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryEnabledBySceneFiltersEnabledAndActive(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_billing_rules` WHERE is_del = ? AND scene = ? AND status = ? ORDER BY `ai_billing_rules`.`id` LIMIT ?")).
		WithArgs(enum.CommonNo, SceneAdminImageGenerate, RuleStatusEnabled, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "unit", "unit_price_cents", "status", "is_del", "created_at", "updated_at"}).
			AddRow(uint64(1), SceneAdminImageGenerate, UnitRequest, int64(100), RuleStatusEnabled, enum.CommonNo, now, now))

	rule, err := repo.EnabledByScene(context.Background(), SceneAdminImageGenerate)
	if err != nil {
		t.Fatalf("EnabledByScene error=%v", err)
	}
	if rule == nil || rule.ID != 1 || rule.Status != RuleStatusEnabled || rule.IsDel != enum.CommonNo {
		t.Fatalf("unexpected rule=%#v", rule)
	}
	assertMockExpectations(t, mock)
}

func TestBillingRecordModelDoesNotUseSoftDeleteColumn(t *testing.T) {
	recordType := reflect.TypeOf(BillingRecord{})
	for i := 0; i < recordType.NumField(); i++ {
		if recordType.Field(i).Tag.Get("gorm") == "column:is_del" {
			t.Fatalf("ai_billing_records must not define is_del: field=%s", recordType.Field(i).Name)
		}
	}
}

func TestRepositoryListUsesActiveFilterAndPagination(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `ai_billing_rules` WHERE is_del = ? AND scene = ? AND status = ?")).
		WithArgs(enum.CommonNo, SceneAdminImageGenerate, RuleStatusEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_billing_rules` WHERE is_del = ? AND scene = ? AND status = ? ORDER BY id DESC LIMIT ? OFFSET ?")).
		WithArgs(enum.CommonNo, SceneAdminImageGenerate, RuleStatusEnabled, 10, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "unit", "unit_price_cents", "status", "is_del", "created_at", "updated_at"}))

	_, total, err := repo.List(context.Background(), ListQuery{CurrentPage: 2, PageSize: 10, Scene: SceneAdminImageGenerate, Status: ptrInt(RuleStatusEnabled)})
	if err != nil {
		t.Fatalf("List error=%v", err)
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
	assertMockExpectations(t, mock)
}

func newMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm mock db: %v", err)
	}
	return NewGormRepository(&database.Client{Gorm: db, SQL: sqlDB}), mock, func() { _ = sqlDB.Close() }
}

func assertMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func ptrInt(v int) *int { return &v }
