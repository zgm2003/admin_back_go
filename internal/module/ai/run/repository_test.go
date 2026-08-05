package airun

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestRunModelMapsLikedAt(t *testing.T) {
	parsed, err := schema.Parse(&Run{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Run schema: %v", err)
	}
	field := parsed.LookUpField("LikedAt")
	if field == nil {
		t.Fatal("Run must persist LikedAt")
	}
	if field.DBName != "liked_at" {
		t.Fatalf("LikedAt column mismatch: %q", field.DBName)
	}
	if field.FieldType != reflect.TypeOf((*time.Time)(nil)) {
		t.Fatalf("LikedAt type mismatch: %v", field.FieldType)
	}
}

func TestRunDetailRowMarksMessageSummariesIgnoredByGorm(t *testing.T) {
	_, err := schema.Parse(&RunDetailRow{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("message summaries are response-only fields and must not be parsed as gorm relations: %v", err)
	}
}

func TestRunDetailReadsErrorCodeFromSameFinalAttemptAsList(t *testing.T) {
	db, mock, closeDB := newRunRepositorySQLMock(t)
	defer closeDB()
	repository := &GormRepository{db: db}

	mock.ExpectQuery(`(?s)SELECT .*COALESCE\(final_attempt\.error_code, ''\) AS error_code.*FROM ai_runs r.*LEFT JOIN ai_provider_attempts final_attempt.*newer_attempt\.attempt_no > final_attempt\.attempt_no.*WHERE r\.id = \?`).
		WithArgs(int64(44)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "error_code"}))

	if _, err := repository.Detail(context.Background(), 44); err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("detail query must project the final terminal attempt error code: %v", err)
	}
}

func TestInputSnapshotReadsOnlyPersistedRunEvidence(t *testing.T) {
	db, mock, closeDB := newRunRepositorySQLMock(t)
	defer closeDB()
	repository := &GormRepository{db: db}
	const snapshot = `{"content":"describe","attachments":[]}`

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id AS run_id, input_snapshot FROM `ai_runs` WHERE id = ?")).
		WithArgs(int64(44)).
		WillReturnRows(sqlmock.NewRows([]string{"run_id", "input_snapshot"}).AddRow(44, snapshot))

	row, err := repository.InputSnapshot(context.Background(), 44)
	if err != nil {
		t.Fatalf("InputSnapshot returned error: %v", err)
	}
	if row == nil || row.RunID != 44 || row.InputSnapshot != snapshot {
		t.Fatalf("input snapshot row=%+v", row)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("input snapshot query must remain bounded: %v", err)
	}
}

func TestBillingDetailUsesThreeBoundedQueries(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	repo := &GormRepository{db: db}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, held_units, actual_units, status FROM `ai_usage_charges` WHERE run_id = ? LIMIT ?")).WithArgs(int64(44), 1).WillReturnRows(
		sqlmock.NewRows([]string{"id", "held_units", "actual_units", "status"}).AddRow(9, 900, 250, "settled"),
	)
	mock.ExpectQuery("SELECT .* FROM ai_usage_charge_items i JOIN ai_usage_charges c ON c.id = i.charge_id JOIN ai_provider_attempts a ON a.id = i.attempt_id AND a.run_id = c.run_id WHERE c.run_id = \\? ORDER BY a.attempt_no ASC, i.id ASC").WithArgs(int64(44)).WillReturnRows(
		sqlmock.NewRows([]string{"attempt_id", "attempt_no", "attempt_state", "category", "tier_key", "quantity", "unit", "unit_price_units", "unit_scale", "amount_units"}).AddRow(101, 1, "succeeded", "input", "", 2, "token", 100, 1, 250),
	)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, attempt_no, state, provider_request_id, usage_status, usage_json, prepared_request_json, prepare_started_at, dispatched_at, first_delta_at, finished_at FROM `ai_provider_attempts` WHERE run_id = ? ORDER BY attempt_no ASC")).WithArgs(int64(44)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "attempt_no", "state", "provider_request_id", "usage_status", "usage_json", "prepared_request_json", "prepare_started_at", "dispatched_at", "first_delta_at", "finished_at"}).
			AddRow(101, 1, "succeeded", "provider-1", "complete", `{"status":"complete","items":[]}`, `{"messages":[]}`, nil, nil, nil, nil),
	)

	charge, items, attempts, err := repo.BillingDetail(context.Background(), 44)
	if err != nil {
		t.Fatalf("BillingDetail returned error: %v", err)
	}
	if charge == nil || charge.HeldUnits != 900 || len(items) != 1 || items[0].AttemptNo != 1 || len(attempts) != 1 || attempts[0].ProviderRequestID != "provider-1" {
		t.Fatalf("unexpected billing facts: charge=%#v items=%#v attempts=%#v", charge, items, attempts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("billing detail must use exactly the three expected set queries: %v", err)
	}
}

func TestPageInitHistoricalModelsUseRequestedDateRangeAndLatestSnapshot(t *testing.T) {
	db, mock, closeDB := newRunRepositorySQLMock(t)
	defer closeDB()
	repository := &GormRepository{db: db}
	location, err := time.LoadLocation(dashboardTimezone)
	if err != nil {
		t.Fatal(err)
	}
	startAt := time.Date(2026, 7, 28, 0, 0, 0, 0, location)
	endExclusive := time.Date(2026, 7, 30, 0, 0, 0, 0, location)
	mock.ExpectQuery(`(?s)SELECT model_id, model_display_name.*ROW_NUMBER\(\) OVER \(PARTITION BY model_id ORDER BY created_at DESC, id DESC\) AS row_no.*created_at >= \? AND created_at < \?.*WHERE row_no = 1.*ORDER BY model_id ASC`).
		WithArgs(startAt, endExclusive).
		WillReturnRows(sqlmock.NewRows([]string{"model_id", "model_display_name"}).AddRow("legacy-model", "最新名称"))

	rows, queryErr := repository.HistoricalModelOptions(context.Background(), startAt, endExclusive)
	if queryErr != nil {
		t.Fatalf("HistoricalModelOptions returned error: %v", queryErr)
	}
	if len(rows) != 1 || rows[0].ModelID != "legacy-model" || rows[0].ModelDisplayName != "最新名称" {
		t.Fatalf("historical models=%+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("historical model query must use latest in-range Run snapshot: %v", err)
	}
}

func TestRunListErrorFilterDoesNotDuplicateRunsWithRetries(t *testing.T) {
	sql := renderRunListQuerySQL(t, ListQuery{ErrorCode: "provider_timeout"})
	assertDashboardSQLContains(t, sql,
		"left join ai_provider_attempts final_attempt on final_attempt.run_id = r.id",
		"final_attempt.state in ('succeeded', 'failed', 'canceled', 'outcome_unknown')",
		"not exists (",
		"select 1 from ai_provider_attempts newer_attempt",
		"newer_attempt.attempt_no > final_attempt.attempt_no",
		"newer_attempt.attempt_no = final_attempt.attempt_no and newer_attempt.id > final_attempt.id",
		"r.status in (?,?,?)",
		"coalesce(nullif(trim(final_attempt.error_code), ''), 'unclassified') = ?",
	)
	if strings.Contains(sql, " distinct ") {
		t.Fatalf("run list must prevent retry duplication structurally, sql=%s", sql)
	}
}

func TestRunListFiltersRunsContainingToolCodeWithoutDuplicateRows(t *testing.T) {
	sql := renderRunListQuerySQL(t, ListQuery{ToolCode: "lookup"})
	assertDashboardSQLContains(t, sql,
		"exists (select 1 from ai_tool_calls tc where tc.run_id = r.id and tc.tool_code = ?)",
	)
	if strings.Contains(sql, "join ai_tool_calls") || strings.Contains(sql, "join ai_usage_charges") || strings.Contains(sql, " distinct ") {
		t.Fatalf("tool filter must use EXISTS without duplicate-hiding DISTINCT, sql=%s", sql)
	}
}

func TestRunListProjectsAndFiltersPersistedUserFeedback(t *testing.T) {
	for _, test := range []struct {
		name         string
		userFeedback string
		wantSQL      string
	}{
		{name: "liked", userFeedback: "liked", wantSQL: "r.liked_at is not null"},
		{name: "unliked", userFeedback: "unliked", wantSQL: "r.liked_at is null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sql := renderRunListQuerySQL(t, ListQuery{UserFeedback: test.userFeedback})
			assertDashboardSQLContains(t, sql, "r.liked_at", test.wantSQL)
		})
	}
}

func TestRunListBillingAnomalyBindsEachPlaceholderOnce(t *testing.T) {
	staleBefore := time.Date(2026, 7, 30, 9, 45, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	statement := renderRunListQueryStatement(t, ListQuery{
		BillingAnomaly: "state_inconsistent",
		StaleBefore:    staleBefore,
	})
	if placeholders, bindings := strings.Count(statement.SQL.String(), "?"), len(statement.Vars); placeholders != bindings {
		t.Fatalf("billing anomaly query placeholders=%d bindings=%d sql=%s vars=%#v", placeholders, bindings, statement.SQL.String(), statement.Vars)
	}
}

func newRunRepositorySQLMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return db, mock, func() { _ = sqlDB.Close() }
}

func renderRunListQuerySQL(t *testing.T, query ListQuery) string {
	t.Helper()
	return normalizeDashboardSQL(renderRunListQueryStatement(t, query).SQL.String())
}

func renderRunListQueryStatement(t *testing.T, query ListQuery) *gorm.Statement {
	t.Helper()
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := &GormRepository{db: db}
	return applyListFilters(repository.listBase(context.Background(), query, true), query).
		Select(runListSelectSQL()).Find(&[]ListRow{}).Statement
}

func sqlSummaryLower(sql string) string {
	return strings.ToLower(strings.Join(strings.Fields(sql), " "))
}
