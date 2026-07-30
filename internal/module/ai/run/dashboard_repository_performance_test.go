package airun

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestDashboardPerformanceEvidence(t *testing.T) {
	if strings.TrimSpace(os.Getenv("AI_RUN_DASHBOARD_PERF")) != "1" {
		t.Skip("AI_RUN_DASHBOARD_PERF=1 is required for dashboard performance evidence")
	}
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is required for dashboard performance evidence")
	}
	configuration, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}
	if configuration.DBName != "admin_ai_dashboard_perf" {
		t.Fatalf("TEST_MYSQL_DSN must target admin_ai_dashboard_perf, got %q", configuration.DBName)
	}
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	configuration.ParseTime = true
	configuration.Loc = shanghai
	db, err := gorm.Open(gormmysql.Open(configuration.FormatDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open dashboard performance database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var fixtureRuns int64
	if err := db.Table("ai_runs").Where("request_id LIKE ?", "dashboard-perf-%").Count(&fixtureRuns).Error; err != nil {
		t.Fatalf("count dashboard performance fixture: %v", err)
	}
	if fixtureRuns != 100000 {
		t.Fatalf("dashboard performance fixture runs=%d want 100000", fixtureRuns)
	}
	assertDashboardProjectionClosure(t, db)

	query := DashboardQuery{
		StartAt:      time.Date(2026, 5, 1, 0, 0, 0, 0, shanghai),
		EndExclusive: time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai),
		GeneratedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, shanghai),
		StaleBefore:  time.Date(2026, 7, 29, 11, 45, 0, 0, shanghai),
	}
	repository := &GormRepository{db: db}
	logDashboardStageDurations(t, db, query)
	if stage := strings.TrimSpace(os.Getenv("AI_RUN_DASHBOARD_EXPLAIN")); stage != "" {
		logDashboardStagePlan(t, db, query, DashboardQueryStage(stage))
	}
	durations := make([]time.Duration, 0, 4)
	for run := 1; run <= 5; run++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		startedAt := time.Now()
		_, queryErr := repository.Dashboard(ctx, query)
		duration := time.Since(startedAt)
		cancel()
		if queryErr != nil {
			t.Fatalf("dashboard performance run %d: %v", run, queryErr)
		}
		if run == 1 {
			t.Logf("dashboard warmup duration=%s", duration)
			continue
		}
		durations = append(durations, duration)
		t.Logf("dashboard hot run %d duration=%s", run, duration)
	}
	p95 := nearestRankDuration(durations, 95)
	t.Logf("dashboard hot nearest-rank p95=%s", p95)
	if p95 >= 500*time.Millisecond {
		t.Fatalf("dashboard hot p95=%s want <500ms", p95)
	}
}

func assertDashboardProjectionClosure(t *testing.T, db *gorm.DB) {
	t.Helper()
	var closure struct {
		TerminalRuns     int64
		FactRuns         int64
		DailyRuns        int64
		FactActualUnits  int64
		DailyActualUnits int64
		FactTotalTokens  int64
		DailyTotalTokens int64
	}
	err := db.Raw(`
SELECT
  (SELECT COUNT(*) FROM ai_runs WHERE status IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')) AS terminal_runs,
  (SELECT COUNT(*) FROM ai_run_dashboard_facts) AS fact_runs,
  (SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts) AS daily_runs,
  (SELECT COALESCE(SUM(actual_units), 0) FROM ai_run_dashboard_facts) AS fact_actual_units,
  (SELECT COALESCE(SUM(actual_units), 0) FROM ai_run_dashboard_daily_facts) AS daily_actual_units,
  (SELECT COALESCE(SUM(total_tokens), 0) FROM ai_run_dashboard_facts) AS fact_total_tokens,
  (SELECT COALESCE(SUM(total_tokens), 0) FROM ai_run_dashboard_daily_facts) AS daily_total_tokens`).Scan(&closure).Error
	if err != nil {
		t.Fatalf("query dashboard projection closure: %v", err)
	}
	if closure.TerminalRuns != closure.FactRuns || closure.FactRuns != closure.DailyRuns {
		t.Fatalf("dashboard projection run closure: terminal=%d facts=%d daily=%d", closure.TerminalRuns, closure.FactRuns, closure.DailyRuns)
	}
	if closure.FactActualUnits != closure.DailyActualUnits {
		t.Fatalf("dashboard projection amount closure: facts=%d daily=%d", closure.FactActualUnits, closure.DailyActualUnits)
	}
	if closure.FactTotalTokens != closure.DailyTotalTokens {
		t.Fatalf("dashboard projection token closure: facts=%d daily=%d", closure.FactTotalTokens, closure.DailyTotalTokens)
	}
	t.Logf(
		"dashboard projection closure runs=%d actual_units=%d total_tokens=%d",
		closure.FactRuns,
		closure.FactActualUnits,
		closure.FactTotalTokens,
	)
}

func logDashboardStagePlan(t *testing.T, db *gorm.DB, query DashboardQuery, stage DashboardQueryStage) {
	t.Helper()
	builders := map[DashboardQueryStage]func(*gorm.DB, DashboardQuery) *gorm.DB{
		DashboardStageOverview:     dashboardOverviewQuery,
		DashboardStagePerformance:  dashboardPerformanceQuery,
		DashboardStageTrend:        dashboardTrendQuery,
		DashboardStageAttributions: dashboardAttributionsQuery,
		DashboardStageErrors:       dashboardErrorsQuery,
		DashboardStageTools:        dashboardToolsQuery,
	}
	build, exists := builders[stage]
	if !exists {
		t.Fatalf("unsupported dashboard explain stage %q", stage)
	}
	statement := build(db.Session(&gorm.Session{DryRun: true}), query).Statement
	if statement.SQL.Len() == 0 {
		t.Fatalf("dashboard explain stage %q returned empty SQL", stage)
	}
	executableSQL := db.Dialector.Explain(statement.SQL.String(), statement.Vars...)
	var rows []struct {
		Plan string `gorm:"column:EXPLAIN"`
	}
	if err := db.Raw("EXPLAIN ANALYZE FORMAT=TREE " + executableSQL).Scan(&rows).Error; err != nil {
		t.Fatalf("explain dashboard stage %q: %v", stage, err)
	}
	for _, row := range rows {
		t.Logf("dashboard explain stage=%s %s", stage, row.Plan)
	}
}

func logDashboardStageDurations(t *testing.T, db *gorm.DB, query DashboardQuery) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result DashboardRepositoryResult
	stages := []struct {
		name string
		run  func(*gorm.DB) error
	}{
		{name: string(DashboardStageOverview), run: func(tx *gorm.DB) error { return scanDashboardOverview(tx, query, &result) }},
		{name: string(DashboardStagePerformance), run: func(tx *gorm.DB) error { return scanDashboardPerformance(tx, query, &result) }},
		{name: string(DashboardStageTrend), run: func(tx *gorm.DB) error { return scanDashboardTrend(tx, query, &result) }},
		{name: string(DashboardStageAttributions), run: func(tx *gorm.DB) error { return scanDashboardAttributions(tx, query, &result) }},
		{name: string(DashboardStageErrors), run: func(tx *gorm.DB) error { return dashboardErrorsQuery(tx, query).Scan(&result.Errors).Error }},
		{name: string(DashboardStageTools), run: func(tx *gorm.DB) error { return scanDashboardTools(tx, query, &result) }},
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stage := range stages {
			startedAt := time.Now()
			if err := stage.run(tx); err != nil {
				return &DashboardQueryError{Stage: DashboardQueryStage(stage.name), Err: err}
			}
			t.Logf("dashboard stage=%s duration=%s", stage.name, time.Since(startedAt))
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatalf("profile dashboard stages: %v", err)
	}
}

func nearestRankDuration(values []time.Duration, percentile int) time.Duration {
	if len(values) == 0 || percentile <= 0 || percentile > 100 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (percentile*len(sorted)+99)/100 - 1
	return sorted[index]
}
