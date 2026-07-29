package airun

import (
	"context"
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

	query := DashboardQuery{
		StartAt:      time.Date(2026, 5, 1, 0, 0, 0, 0, shanghai),
		EndExclusive: time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai),
		GeneratedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, shanghai),
		StaleBefore:  time.Date(2026, 7, 29, 11, 45, 0, 0, shanghai),
	}
	repository := &GormRepository{db: db}
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

func nearestRankDuration(values []time.Duration, percentile int) time.Duration {
	if len(values) == 0 || percentile <= 0 || percentile > 100 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (percentile*len(sorted)+99)/100 - 1
	return sorted[index]
}
