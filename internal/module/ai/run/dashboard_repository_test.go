package airun

import (
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func TestDashboardQueriesDoNotSelectLargeJSONColumns(t *testing.T) {
	queries := map[string]string{
		"overview":    renderDashboardQuerySQL(t, dashboardOverviewQuery),
		"performance": renderDashboardQuerySQL(t, dashboardPerformanceQuery),
		"trend":       renderDashboardQuerySQL(t, dashboardTrendQuery),
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
