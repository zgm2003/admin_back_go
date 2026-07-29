package airun

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	dashboardOverviewQueryPattern     = `(?i)WITH filtered_runs AS .*overview AS`
	dashboardPerformanceQueryPattern  = `(?i)WITH filtered_runs AS .*performance_samples AS`
	dashboardTrendQueryPattern        = `(?i)WITH filtered_runs AS .*daily_runs AS`
	dashboardAttributionsQueryPattern = `(?i)WITH filtered_runs AS .*model_attributions AS`
	dashboardErrorsQueryPattern       = `(?i)WITH filtered_runs AS .*ranked_terminal_attempts AS`
	dashboardToolsQueryPattern        = `(?i)WITH filtered_runs AS .*filtered_tool_calls AS`
)

func TestDashboardOverviewUsesTerminalDeliveryAndSettledChargeFacts(t *testing.T) {
	sql := renderDashboardQuerySQL(t, dashboardOverviewQuery)

	assertDashboardSQLContains(t, sql,
		"r.created_at >= ? and r.created_at < ?",
		"charge.id is null",
		"r.status = 'running' and r.billing_status in ('settled', 'released', 'unbilled')",
		"r.status in ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')",
		"r.started_at < ?",
		"r.billing_status = 'settled'",
		"charge.status = 'settled' and charge.finalized_at is not null",
		"r.billing_status = 'released'",
		"charge.status = 'released' and charge.finalized_at is not null",
		"r.billing_status = 'unbilled'",
		"charge.pricing_version",
		"r.billing_status <> 'released' and (charge.pricing_version is null or trim(charge.pricing_version) = '')",
	)

	matrix := []string{
		"r.billing_status = 'pending' and r.billing_reason = 'pending' and charge.status = 'open' and charge.finalized_at is null",
		"r.billing_status = 'held' and r.billing_reason = 'held' and charge.status = 'open' and charge.finalized_at is null",
		"r.billing_status = 'settled' and r.billing_reason = 'settled_complete_usage' and charge.status = 'settled' and charge.finalized_at is not null",
		"r.billing_status = 'released' and r.billing_reason in ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown') and charge.status = 'released' and charge.finalized_at is not null",
		"r.billing_status = 'unbilled' and r.billing_reason in ('legacy_unpriced', 'unbilled_usage_incomplete', 'unbilled_over_hold') and charge.status = 'unbilled' and charge.finalized_at is not null",
	}
	for _, clause := range matrix {
		assertDashboardSQLContains(t, sql, clause)
	}

	orderedCodes := []string{
		"then 'state_inconsistent'",
		"then 'open_overdue'",
		"then 'pricing_snapshot_missing'",
		"then 'legacy_unpriced'",
		"then 'unbilled_usage_incomplete'",
		"then 'unbilled_over_hold'",
	}
	last := -1
	for _, code := range orderedCodes {
		index := strings.Index(sql, code)
		if index < 0 || index <= last {
			t.Fatalf("billing anomaly CASE order is invalid at %q, sql=%s", code, sql)
		}
		last = index
	}
	if strings.Contains(sql, "then 'released'") {
		t.Fatalf("normal released billing must not be classified as an anomaly, sql=%s", sql)
	}
	if strings.Count(sql, "union all") != 2 {
		t.Fatalf("overview must return summary and both anomaly groups in one statement, sql=%s", sql)
	}
}

func TestDashboardPerformanceUsesSuccessfulRunsAndNearestRank(t *testing.T) {
	sql := renderDashboardQuerySQL(t, dashboardPerformanceQuery)

	assertDashboardSQLContains(t, sql,
		"r.created_at >= ? and r.created_at < ?",
		"r.status = 'success'",
		"attempt.state = 'succeeded'",
		"row_number() over (partition by attempt.run_id order by attempt.attempt_no desc, attempt.id desc)",
		"final_rank = 1",
		"attempt.first_delta_at >= attempt.dispatched_at",
		"r.duration_ms >= 0",
		"row_number() over (partition by metric order by value_ms asc)",
		"count(*) over (partition by metric)",
		"ceil(0.50 * sample_count)",
		"ceil(0.95 * sample_count)",
	)
	if strings.Contains(sql, "p99") || strings.Contains(sql, "0.99") {
		t.Fatalf("dashboard performance only publishes P50/P95, sql=%s", sql)
	}
}

func TestDashboardTrendUsesShanghaiDayBucketsAndNinetyRowLimit(t *testing.T) {
	sql := renderDashboardQuerySQL(t, dashboardTrendQuery)

	assertDashboardSQLContains(t, sql,
		"r.created_at >= ? and r.created_at < ?",
		"date(r.created_at)",
		"date_format(daily_runs.run_date, '%y-%m-%d')",
		"r.status = 'success'",
		"attempt.state = 'succeeded'",
		"ceil(0.50 * sample_count)",
		"ceil(0.95 * sample_count)",
		"order by daily_runs.run_date asc",
		"limit 90",
	)
	for _, forbidden := range []string{"convert_tz", "date_add", "+ interval 8 hour", "+08:00"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("persisted DATETIME is already an Asia/Shanghai wall clock; found %q in sql=%s", forbidden, sql)
		}
	}
}

func TestDashboardAttributionsUseFourUnionedDimensionsAndTopTwenty(t *testing.T) {
	sql := renderDashboardQuerySQL(t, dashboardAttributionsQuery)

	assertDashboardSQLContains(t, sql,
		"r.created_at >= ? and r.created_at < ?",
		"row_number() over (partition by r.model_id order by r.created_at desc, r.id desc) as model_name_rank",
		"group by r.model_id",
		"max(case when r.model_name_rank = 1 then r.model_display_name end)",
		"'model' as dimension",
		"'provider' as dimension",
		"'agent' as dimension",
		"'user' as dimension",
	)
	if strings.Count(sql, "union all") != 3 {
		t.Fatalf("attribution query must union exactly four dimensions, sql=%s", sql)
	}
	if strings.Count(sql, "order by actual_units desc, total_runs desc, stable_key asc limit 20") != 4 {
		t.Fatalf("each attribution dimension must independently select its top twenty, sql=%s", sql)
	}
	if strings.Contains(sql, "group by r.model_id, r.model_display_name") || strings.Contains(sql, "ai_provider_models") {
		t.Fatalf("model attribution must preserve canonical Run snapshots without current provider-model configuration, sql=%s", sql)
	}
}

func TestDashboardErrorsUseLastTerminalAttemptOnly(t *testing.T) {
	sql := renderDashboardQuerySQL(t, dashboardErrorsQuery)

	assertDashboardSQLContains(t, sql,
		"r.status in ('failed', 'timeout', 'outcome_unknown')",
		"attempt.state in ('succeeded', 'failed', 'canceled', 'outcome_unknown')",
		"row_number() over (partition by attempt.run_id order by attempt.attempt_no desc, attempt.id desc) as final_rank",
		"final_rank = 1",
		"coalesce(nullif(trim(error_code), ''), 'unclassified')",
		"group by coalesce(nullif(trim(error_code), ''), 'unclassified')",
	)
	if strings.Contains(sql, "error_message") {
		t.Fatalf("dashboard errors must not group by unstable messages, sql=%s", sql)
	}
}

func TestDashboardToolsExcludeRunningAndUseSuccessfulDurations(t *testing.T) {
	sql := renderDashboardQuerySQL(t, dashboardToolsQuery)

	assertDashboardSQLContains(t, sql,
		"join ai_tool_calls tool_call on tool_call.run_id = r.id",
		"row_number() over (partition by tool_call.tool_code order by tool_call.started_at desc, tool_call.id desc) as tool_name_rank",
		"max(case when tool_name_rank = 1 then tool_name end)",
		"coalesce(sum(case when status = 'success' then 1 else 0 end), 0) as success_calls",
		"coalesce(sum(case when status = 'failed' then 1 else 0 end), 0) as failed_calls",
		"coalesce(sum(case when status = 'timeout' then 1 else 0 end), 0) as timeout_calls",
		"where status = 'success' and duration_ms is not null and duration_ms >= 0",
		"row_number() over (partition by tool_code order by duration_ms asc)",
		"ceil(0.50 * sample_count)",
		"ceil(0.95 * sample_count)",
		"order by total_calls desc, tool_code asc",
		"limit 20",
	)
	if strings.Contains(sql, "ai_tools") || strings.Contains(sql, "status = 'running' then 1") {
		t.Fatalf("tool attribution must use call snapshots and exclude running from its success denominator, sql=%s", sql)
	}
}

func TestDashboardUsesExactlySixQueriesInOneReadOnlyRepeatableReadTransaction(t *testing.T) {
	db, mock, transactionOptions := newDashboardRepositoryTestDB(t)
	mock.ExpectBegin()
	expectSuccessfulDashboardQueries(mock)
	mock.ExpectCommit()

	result, err := (&GormRepository{db: db}).Dashboard(context.Background(), dashboardRepositoryTestQuery(t))
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if result.Summary.TotalRuns != 9 || result.Billing.ActualUnits != 100 ||
		result.Performance.TTFT != (DashboardDistributionRow{SampleCount: 21, P50MS: 10, P95MS: 20}) ||
		result.Performance.EndToEnd != (DashboardDistributionRow{SampleCount: 22, P50MS: 30, P95MS: 40}) {
		t.Fatalf("overview/performance mapping=%+v", result)
	}
	if len(result.Trend) != 1 || result.Trend[0].TTFT.P95MS != 20 || result.Trend[0].EndToEnd.P95MS != 40 ||
		len(result.Attributions) != 1 || result.Attributions[0].Key != "gpt-5.5" || result.Attributions[0].Name != "GPT-5.5" ||
		len(result.Errors) != 1 || result.Errors[0] != (DashboardErrorRow{ErrorCode: "provider_timeout", Count: 2}) ||
		len(result.Tools) != 1 || result.Tools[0].ToolName != "Lookup" || result.Tools[0].Duration.P95MS != 18 {
		t.Fatalf("trend/breakdown mapping=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dashboard did not execute exactly six ordered queries in one transaction: %v", err)
	}
	if len(*transactionOptions) != 1 {
		t.Fatalf("transaction option calls=%d want=1", len(*transactionOptions))
	}
	options := (*transactionOptions)[0]
	if !options.ReadOnly || options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
		t.Fatalf("transaction options=%+v want read-only repeatable-read", options)
	}
}

func TestDashboardRollsBackAndReturnsNoPartialResultWhenAnyQueryFails(t *testing.T) {
	db, mock, _ := newDashboardRepositoryTestDB(t)
	queryErr := errors.New("error_stage_unavailable")
	mock.ExpectBegin()
	mock.ExpectQuery(dashboardOverviewQueryPattern).WillReturnRows(dashboardOverviewRows().AddRow(
		"summary", "", 0, 9, 0, 8, 1, 0, 0, 0, 10, 11, 21, 1, 100, 0, 0, 0,
	))
	mock.ExpectQuery(dashboardPerformanceQueryPattern).WillReturnRows(dashboardPerformanceRows())
	mock.ExpectQuery(dashboardTrendQueryPattern).WillReturnRows(dashboardTrendRows())
	mock.ExpectQuery(dashboardAttributionsQueryPattern).WillReturnRows(dashboardAttributionRows().AddRow(
		"model", "historical-model", 0, "Historical Model", 9, 8, 1, 0, 0, 21, 100, 1, 0,
	))
	mock.ExpectQuery(dashboardErrorsQueryPattern).WillReturnError(queryErr)
	mock.ExpectRollback()

	service := NewService(&GormRepository{db: db})
	response, appErr := service.Dashboard(context.Background(), DashboardFilter{})
	if response != nil || appErr == nil || !errors.Is(appErr.Cause, queryErr) {
		t.Fatalf("response=%#v appErr=%#v", response, appErr)
	}
	var dashboardErr *DashboardQueryError
	if !errors.As(appErr.Cause, &dashboardErr) || dashboardErr.Stage != DashboardStageErrors {
		t.Fatalf("query error=%#v want errors stage", appErr.Cause)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dashboard must stop at the failed stage and roll back: %v", err)
	}
}

func TestDashboardQueriesDoNotSelectLargeJSONColumns(t *testing.T) {
	queries := map[string]string{
		"overview":     renderDashboardQuerySQL(t, dashboardOverviewQuery),
		"performance":  renderDashboardQuerySQL(t, dashboardPerformanceQuery),
		"trend":        renderDashboardQuerySQL(t, dashboardTrendQuery),
		"attributions": renderDashboardQuerySQL(t, dashboardAttributionsQuery),
		"errors":       renderDashboardQuerySQL(t, dashboardErrorsQuery),
		"tools":        renderDashboardQuerySQL(t, dashboardToolsQuery),
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			sql := normalizeDashboardSQL(query)
			for _, forbidden := range []string{
				"input_snapshot", "pricing_snapshot_json", "prepared_request_json", "usage_json",
				"arguments_json", "result_json", "select *", "limit 10000",
			} {
				if strings.Contains(sql, forbidden) {
					t.Fatalf("dashboard query must not read %q, sql=%s", forbidden, sql)
				}
			}
		})
	}
}

func newDashboardRepositoryTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *[]driver.TxOptions) {
	t.Helper()
	dsn := fmt.Sprintf("dashboard_%s_%d", strings.ReplaceAll(t.Name(), "/", "_"), time.Now().UnixNano())
	seedDB, mock, err := sqlmock.NewWithDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	transactionOptions := make([]driver.TxOptions, 0, 1)
	connector := &dashboardRecordingConnector{
		driver:             seedDB.Driver(),
		dsn:                dsn,
		transactionOptions: &transactionOptions,
	}
	sqlDB := sql.OpenDB(connector)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = seedDB.Close()
	})
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock, &transactionOptions
}

type dashboardRecordingConnector struct {
	driver             driver.Driver
	dsn                string
	transactionOptions *[]driver.TxOptions
}

func (connector *dashboardRecordingConnector) Connect(context.Context) (driver.Conn, error) {
	connection, err := connector.driver.Open(connector.dsn)
	if err != nil {
		return nil, err
	}
	return &dashboardRecordingConn{Conn: connection, transactionOptions: connector.transactionOptions}, nil
}

func (connector *dashboardRecordingConnector) Driver() driver.Driver { return connector.driver }

type dashboardRecordingConn struct {
	driver.Conn
	transactionOptions *[]driver.TxOptions
}

func (connection *dashboardRecordingConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	*connection.transactionOptions = append(*connection.transactionOptions, options)
	return connection.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
}

func (connection *dashboardRecordingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return connection.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (connection *dashboardRecordingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return connection.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (connection *dashboardRecordingConn) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := connection.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return nil
}

func expectSuccessfulDashboardQueries(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(dashboardOverviewQueryPattern).WillReturnRows(dashboardOverviewRows().AddRow(
		"summary", "", 0, 9, 0, 8, 1, 0, 0, 0, 10, 11, 21, 1, 100, 0, 0, 0,
	))
	mock.ExpectQuery(dashboardPerformanceQueryPattern).WillReturnRows(dashboardPerformanceRows().
		AddRow("ttft", 21, 10, 20).
		AddRow("end_to_end", 22, 30, 40))
	mock.ExpectQuery(dashboardTrendQueryPattern).WillReturnRows(dashboardTrendRows().AddRow(
		"2026-07-29", 1, 0, 1, 0, 0, 0, 0, 100, 21, 10, 20, 22, 30, 40,
	))
	mock.ExpectQuery(dashboardAttributionsQueryPattern).WillReturnRows(dashboardAttributionRows().AddRow(
		"model", "gpt-5.5", 0, "GPT-5.5", 9, 8, 1, 0, 0, 21, 100, 1, 0,
	))
	mock.ExpectQuery(dashboardErrorsQueryPattern).WillReturnRows(dashboardErrorRows().AddRow("provider_timeout", 2))
	mock.ExpectQuery(dashboardToolsQueryPattern).WillReturnRows(dashboardToolRows().AddRow(
		"lookup", "Lookup", 5, 3, 1, 0, 3, 8, 18,
	))
}

func dashboardOverviewRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"row_type", "code", "count_value", "total_runs", "running_runs", "success_runs", "failed_runs", "canceled_runs",
		"timeout_runs", "outcome_unknown_runs", "prompt_tokens", "completion_tokens", "total_tokens", "settled_runs", "actual_units",
		"released_runs", "released_units", "unbilled_runs",
	})
}

func dashboardPerformanceRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"metric", "sample_count", "p50_ms", "p95_ms"})
}

func dashboardTrendRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"date", "total_runs", "running_runs", "success_runs", "failed_runs", "canceled_runs", "timeout_runs", "outcome_unknown_runs",
		"actual_units", "ttft_sample_count", "ttft_p50_ms", "ttft_p95_ms", "end_to_end_sample_count", "end_to_end_p50_ms", "end_to_end_p95_ms",
	})
}

func dashboardAttributionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"dimension", "attribution_key", "id", "name", "total_runs", "success_runs", "failed_runs", "timeout_runs", "outcome_unknown_runs",
		"total_tokens", "actual_units", "run_anomaly_count", "billing_anomaly_count",
	})
}

func dashboardErrorRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"error_code", "count"})
}

func dashboardToolRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"tool_code", "tool_name", "total_calls", "success_calls", "failed_calls", "timeout_calls",
		"duration_sample_count", "duration_p50_ms", "duration_p95_ms",
	})
}

func dashboardRepositoryTestQuery(t *testing.T) DashboardQuery {
	t.Helper()
	location, err := time.LoadLocation(dashboardTimezone)
	if err != nil {
		t.Fatal(err)
	}
	return DashboardQuery{
		StartAt:      time.Date(2026, 7, 23, 0, 0, 0, 0, location),
		EndExclusive: time.Date(2026, 7, 30, 0, 0, 0, 0, location),
		GeneratedAt:  time.Date(2026, 7, 29, 15, 42, 18, 0, location),
		StaleBefore:  time.Date(2026, 7, 29, 15, 27, 18, 0, location),
	}
}

func renderDashboardQuerySQL(t *testing.T, build func(*gorm.DB, DashboardQuery) *gorm.DB) string {
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
	location, err := time.LoadLocation(dashboardTimezone)
	if err != nil {
		t.Fatal(err)
	}
	agentID, providerID, userID := int64(2), int64(3), int64(4)
	query := DashboardQuery{
		StartAt:      time.Date(2026, 7, 23, 0, 0, 0, 0, location),
		EndExclusive: time.Date(2026, 7, 30, 0, 0, 0, 0, location),
		GeneratedAt:  time.Date(2026, 7, 29, 15, 42, 18, 0, location),
		StaleBefore:  time.Date(2026, 7, 29, 15, 27, 18, 0, location),
		Platform:     "admin",
		ModelID:      "gpt-5.5",
		AgentID:      &agentID,
		ProviderID:   &providerID,
		UserID:       &userID,
	}
	statement := build(db, query).Statement
	if statement.SQL.Len() == 0 {
		t.Fatal("dashboard query builder returned empty SQL")
	}
	return normalizeDashboardSQL(statement.SQL.String())
}

func normalizeDashboardSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func assertDashboardSQLContains(t *testing.T, sql string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(sql, strings.ToLower(fragment)) {
			t.Fatalf("dashboard SQL missing %q, sql=%s", fragment, sql)
		}
	}
}
