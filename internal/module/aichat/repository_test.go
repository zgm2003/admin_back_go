package aichat

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestStaleRunningRunsDBFiltersOnlyOldRunningRows(t *testing.T) {
	db := dryRunGormDB(t)
	staleBefore := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	var rows []Run
	stmt := staleRunningRunsDB(db.Model(&Run{}), staleBefore).
		Order("id ASC").
		Limit(10).
		Find(&rows).Statement

	sqlText := compactSQL(stmt.SQL.String())
	for _, want := range []string{
		"FROM `ai_runs`",
		"status = ?",
		"started_at IS NOT NULL",
		"started_at < ?",
		"ORDER BY id ASC",
		"LIMIT ?",
	} {
		if !strings.Contains(sqlText, want) {
			t.Fatalf("stale running query missing %q: %s", want, sqlText)
		}
	}
	if len(stmt.Vars) != 3 || stmt.Vars[0] != enum.AIRunStatusRunning || !stmt.Vars[1].(time.Time).Equal(staleBefore) {
		t.Fatalf("unexpected stale query vars: %#v", stmt.Vars)
	}
}

func TestRunningRunUpdateDBUsesCompareAndSetStatus(t *testing.T) {
	db := dryRunGormDB(t)

	stmt := runningRunUpdateDB(db, 42).
		Updates(map[string]any{"status": enum.AIRunStatusTimeout}).Statement

	sqlText := compactSQL(stmt.SQL.String())
	for _, want := range []string{
		"UPDATE `ai_runs`",
		"SET `status`=?",
		"WHERE id = ? AND status = ?",
	} {
		if !strings.Contains(sqlText, want) {
			t.Fatalf("finish run update missing %q: %s", want, sqlText)
		}
	}
	if len(stmt.Vars) < 3 || stmt.Vars[len(stmt.Vars)-2] != int64(42) || stmt.Vars[len(stmt.Vars)-1] != enum.AIRunStatusRunning {
		t.Fatalf("unexpected finish update vars: %#v", stmt.Vars)
	}
}

func dryRunGormDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("mysql", "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}
	return db
}

func compactSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
