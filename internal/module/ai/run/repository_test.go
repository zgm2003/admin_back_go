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

func TestStatsSelectsIntegerAverageDuration(t *testing.T) {
	summarySQL := statsSummarySelectSQL()
	groupedSQL := statsGroupedSelectSQL("DATE(r.created_at) as date")

	for name, sql := range map[string]string{
		"summary": sqlSummaryLower(summarySQL),
		"grouped": sqlSummaryLower(groupedSQL),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(sql, "avg_duration_ms") {
				t.Fatalf("average duration alias is required, sql=%s", sql)
			}
			if strings.Contains(sql, "coalesce(avg(r.duration_ms)") {
				t.Fatalf("average duration must not scan raw MySQL AVG decimal into int64, sql=%s", sql)
			}
			if !strings.Contains(sql, "cast(round(avg(r.duration_ms)) as signed)") {
				t.Fatalf("average duration must be rounded and cast before scanning into int64, sql=%s", sql)
			}
		})
	}
}

func TestRepositorySQLUsesAppAndEventSchema(t *testing.T) {
	summarySQL := sqlSummaryLower(statsSummarySelectSQL())
	groupedSQL := sqlSummaryLower(statsGroupedSelectSQL("r.agent_id as agent_id, COALESCE(a.name, '') as agent_name"))

	if !strings.Contains(summarySQL, "r.status in (?, ?, ?)") {
		t.Fatalf("summary must count failed, canceled and timeout as failed terminal runs, sql=%s", summarySQL)
	}
	if !strings.Contains(groupedSQL, "r.agent_id as agent_id") || !strings.Contains(groupedSQL, "agent_name") {
		t.Fatalf("grouped agent stats must expose agent_id/agent_name, sql=%s", groupedSQL)
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

func TestLatencySamplesUsesTerminalBoundedQuery(t *testing.T) {
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
	since := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT .* FROM ai_provider_attempts attempt_row JOIN ai_runs run_row ON run_row.id = attempt_row.run_id LEFT JOIN ai_providers provider_row ON provider_row.id = run_row.provider_id WHERE run_row.created_at >= \\? AND attempt_row.state IN \\(\\?,\\?,\\?,\\?\\) ORDER BY attempt_row.finished_at DESC, attempt_row.id DESC LIMIT \\?").
		WithArgs(since, "succeeded", "failed", "canceled", "outcome_unknown", 10000).
		WillReturnRows(sqlmock.NewRows([]string{"provider_id", "provider_name", "model_id", "dispatched_at", "first_delta_at", "finished_at"}))

	rows, err := repo.LatencySamples(context.Background(), since, 10001)
	if err != nil {
		t.Fatalf("LatencySamples returned error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%#v, want empty result", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("latency samples must use the bounded terminal query: %v", err)
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
	statement := applyListFilters(repository.listBase(context.Background(), query, true), query).
		Select(runListSelectSQL()).Find(&[]ListRow{}).Statement
	return normalizeDashboardSQL(statement.SQL.String())
}

func sqlSummaryLower(sql string) string {
	return strings.ToLower(strings.Join(strings.Fields(sql), " "))
}
