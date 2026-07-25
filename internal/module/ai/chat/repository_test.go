package aichat

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestStaleRunningRunsDBFiltersOnlyOldRunningRows(t *testing.T) {
	db := dryRunGormDB(t)
	staleBefore := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	var rows []Run
	stmt := staleRunningRunsDB(db.Model(&Run{}), staleBefore).
		Limit(10).
		Find(&rows).Statement

	sqlText := compactSQL(stmt.SQL.String())
	for _, want := range []string{
		"FROM `ai_runs`",
		"status = ?",
		"started_at IS NOT NULL",
		"started_at < ?",
		"ORDER BY started_at ASC, id ASC",
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

func TestCreateRunRecordMapsBillingIdentityFacts(t *testing.T) {
	startedAt := time.Date(2026, 7, 25, 20, 0, 0, 0, time.UTC)
	fingerprint := [32]byte{1, 2, 3}
	run := runFromCreateRecord(CreateRunRecord{
		ConversationID:        7,
		RequestID:             " request-9 ",
		RequestFingerprint:    fingerprint,
		RequestIdentityStatus: "replayable",
		RequestIdentityMarker: "",
		UserMessageID:         11,
		UserID:                13,
		AgentID:               17,
		ProviderID:            19,
		ModelID:               " model-1 ",
		ModelDisplayName:      " Model One ",
		PricingSnapshotJSON:   `{"version":"pricing-v1"}`,
		StartedAt:             startedAt,
	}, startedAt)

	if run.RequestID != "request-9" || string(run.RequestFingerprint) != string(fingerprint[:]) {
		t.Fatalf("request identity=%+v", run)
	}
	if run.RequestIdentityStatus != "replayable" || run.RequestIdentityMarker != "" {
		t.Fatalf("identity marker=%+v", run)
	}
	if run.PricingSnapshotJSON != `{"version":"pricing-v1"}` || run.BillingStatus != "pending" || run.BillingReason != "pending" {
		t.Fatalf("billing facts=%+v", run)
	}
}

func TestCreateRunRecordDefaultsReplayableIdentity(t *testing.T) {
	run := runFromCreateRecord(CreateRunRecord{}, time.Now())
	if run.RequestIdentityStatus != "replayable" || run.RequestIdentityMarker != "" {
		t.Fatalf("request identity status=%q marker=%q", run.RequestIdentityStatus, run.RequestIdentityMarker)
	}
	legacy := runFromCreateRecord(CreateRunRecord{
		RequestIdentityStatus: " legacy_non_replayable ",
		RequestIdentityMarker: " legacy-marker ",
	}, time.Now())
	if legacy.RequestIdentityStatus != "legacy_non_replayable" || legacy.RequestIdentityMarker != "legacy-marker" {
		t.Fatalf("legacy request identity status=%q marker=%q", legacy.RequestIdentityStatus, legacy.RequestIdentityMarker)
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
