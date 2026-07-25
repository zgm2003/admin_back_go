package aichat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
	input := validCreateRunRecord()
	input.StartedAt = startedAt
	run, err := runFromCreateRecord(input, startedAt)
	if err != nil {
		t.Fatal(err)
	}

	if run.RequestID != "request-9" || string(run.RequestFingerprint) != string(input.RequestFingerprint[:]) {
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
	input := validCreateRunRecord()
	input.RequestIdentityStatus = ""
	input.RequestIdentityMarker = "   "
	run, err := runFromCreateRecord(input, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.RequestIdentityStatus != "replayable" || run.RequestIdentityMarker != "" {
		t.Fatalf("request identity status=%q marker=%q", run.RequestIdentityStatus, run.RequestIdentityMarker)
	}
	if run.BillingStatus != "pending" || run.BillingReason != "pending" {
		t.Fatalf("billing status=%q reason=%q", run.BillingStatus, run.BillingReason)
	}
}

func TestCreateRunRejectsInvalidBillingIdentityBeforeDatabaseAccess(t *testing.T) {
	tests := map[string]func(*CreateRunRecord){
		"zero fingerprint": func(input *CreateRunRecord) {
			input.RequestFingerprint = [32]byte{}
		},
		"empty pricing": func(input *CreateRunRecord) {
			input.PricingSnapshotJSON = "  "
		},
		"invalid pricing": func(input *CreateRunRecord) {
			input.PricingSnapshotJSON = "{"
		},
		"legacy identity status": func(input *CreateRunRecord) {
			input.RequestIdentityStatus = "legacy_non_replayable"
			input.RequestIdentityMarker = "legacy_non_replayable_v1:ai_runs:9"
		},
		"unknown identity status": func(input *CreateRunRecord) {
			input.RequestIdentityStatus = "unknown"
		},
		"identity marker": func(input *CreateRunRecord) {
			input.RequestIdentityMarker = "runtime-marker"
		},
		"legacy pricing marker": func(input *CreateRunRecord) {
			input.PricingSnapshotJSON = `{"version":"legacy_unpriced_v1","billable":false}`
		},
		"non-billable pricing marker": func(input *CreateRunRecord) {
			input.PricingSnapshotJSON = `{"version":"pricing-v1","billable":false}`
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validCreateRunRecord()
			mutate(&input)
			db, mock := noWriteGormDB(t)
			repository := &GormRepository{db: db}

			id, err := repository.CreateRun(context.Background(), input)
			if id != 0 || !errors.Is(err, ErrInvalidRunBillingIdentity) {
				t.Fatalf("id=%d error=%v", id, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("invalid run reached database: %v", err)
			}
		})
	}
}

func validCreateRunRecord() CreateRunRecord {
	return CreateRunRecord{
		ConversationID:        7,
		RequestID:             " request-9 ",
		RequestFingerprint:    [32]byte{1, 2, 3},
		RequestIdentityStatus: "replayable",
		UserMessageID:         11,
		UserID:                13,
		AgentID:               17,
		ProviderID:            19,
		ModelID:               " model-1 ",
		ModelDisplayName:      " Model One ",
		PricingSnapshotJSON:   `{"version":"pricing-v1"}`,
	}
}

func noWriteGormDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock
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
