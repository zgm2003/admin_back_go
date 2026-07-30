package airun

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestProjectTerminalDashboardFactsIncrementsDailyProjectionOnce(t *testing.T) {
	db, mock, _ := newDashboardRepositoryTestDB(t)
	mock.ExpectQuery(`(?is)SELECT EXISTS\s*\(.*FROM ai_run_dashboard_facts.*run_id = \?`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(false))
	mock.ExpectExec(`(?is)INSERT INTO ai_run_dashboard_facts .* WHERE r.id = \?`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?i)INSERT INTO ai_run_dashboard_daily_facts`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := ProjectTerminalDashboardFacts(context.Background(), db, 42); err != nil {
		t.Fatalf("ProjectTerminalDashboardFacts returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectTerminalDashboardFactsDoesNotDoubleCountReplay(t *testing.T) {
	db, mock, _ := newDashboardRepositoryTestDB(t)
	mock.ExpectQuery(`(?is)SELECT EXISTS\s*\(.*FROM ai_run_dashboard_facts.*run_id = \?`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(true))

	if err := ProjectTerminalDashboardFacts(context.Background(), db, 42); err != nil {
		t.Fatalf("ProjectTerminalDashboardFacts returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectTerminalDashboardFactsTreatsDuplicateKeyAsConcurrentReplay(t *testing.T) {
	db, mock, _ := newDashboardRepositoryTestDB(t)
	mock.ExpectQuery(`(?is)SELECT EXISTS\s*\(.*FROM ai_run_dashboard_facts.*run_id = \?`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(false))
	mock.ExpectExec(`(?is)INSERT INTO ai_run_dashboard_facts .* WHERE r.id = \?`).
		WithArgs(int64(42)).
		WillReturnError(&mysqldriver.MySQLError{Number: 1062, Message: "Duplicate entry '42' for key 'PRIMARY'"})

	if err := ProjectTerminalDashboardFacts(context.Background(), db, 42); err != nil {
		t.Fatalf("ProjectTerminalDashboardFacts returned error for concurrent replay: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectTerminalDashboardFactsPropagatesNonDuplicateConstraintError(t *testing.T) {
	db, mock, _ := newDashboardRepositoryTestDB(t)
	wantErr := &mysqldriver.MySQLError{Number: 3819, Message: "check constraint is violated"}
	mock.ExpectQuery(`(?is)SELECT EXISTS\s*\(.*FROM ai_run_dashboard_facts.*run_id = \?`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"fact_exists"}).AddRow(false))
	mock.ExpectExec(`(?is)INSERT INTO ai_run_dashboard_facts .* WHERE r.id = \?`).
		WithArgs(int64(42)).
		WillReturnError(wantErr)

	err := ProjectTerminalDashboardFacts(context.Background(), db, 42)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ProjectTerminalDashboardFacts error = %v, want %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectTerminalDashboardFactsMySQLReplayEvidence(t *testing.T) {
	db := openDashboardProjectionEvidenceDB(t)

	var runID int64
	if err := db.Raw("SELECT run_id FROM ai_run_dashboard_facts ORDER BY run_id ASC LIMIT 1").Scan(&runID).Error; err != nil || runID <= 0 {
		t.Fatalf("select projection evidence Run: id=%d error=%v", runID, err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	var beforeDailyRuns int64
	if err := tx.Raw("SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts").Scan(&beforeDailyRuns).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("DELETE FROM ai_run_dashboard_facts WHERE run_id = ?", runID).Error; err != nil {
		t.Fatal(err)
	}
	if err := ProjectTerminalDashboardFacts(context.Background(), tx, runID); err != nil {
		t.Fatalf("first projection: %v", err)
	}
	var factCount, afterFirst, afterReplay int64
	if err := tx.Raw("SELECT COUNT(*) FROM ai_run_dashboard_facts WHERE run_id = ?", runID).Scan(&factCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts").Scan(&afterFirst).Error; err != nil {
		t.Fatal(err)
	}
	if factCount != 1 || afterFirst != beforeDailyRuns+1 {
		t.Fatalf("first projection fact_count=%d daily_runs=%d want fact_count=1 daily_runs=%d", factCount, afterFirst, beforeDailyRuns+1)
	}
	if err := ProjectTerminalDashboardFacts(context.Background(), tx, runID); err != nil {
		t.Fatalf("replayed projection: %v", err)
	}
	if err := tx.Raw("SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts").Scan(&afterReplay).Error; err != nil {
		t.Fatal(err)
	}
	if afterReplay != afterFirst {
		t.Fatalf("replayed projection daily_runs=%d want %d", afterReplay, afterFirst)
	}
}

func TestProjectTerminalDashboardFactsMySQLConcurrentReplayEvidence(t *testing.T) {
	db := openDashboardProjectionEvidenceDB(t)

	const candidateSQL = `
SELECT f.run_id
FROM ai_run_dashboard_facts f
JOIN ai_run_dashboard_daily_facts d
  ON d.fact_date = f.fact_date AND d.platform = f.platform AND d.model_id = f.model_id
  AND d.agent_id = f.agent_id AND d.provider_id = f.provider_id AND d.user_id = f.user_id
  AND d.status = f.status AND d.run_anomaly_code = f.run_anomaly_code
  AND d.billing_anomaly_code = f.billing_anomaly_code AND d.final_error_code = f.final_error_code
WHERE d.latest_run_id > f.run_id
ORDER BY f.run_id ASC
LIMIT 1`
	var runID int64
	if err := db.Raw(candidateSQL).Scan(&runID).Error; err != nil || runID <= 0 {
		t.Fatalf("select concurrent projection evidence Run: id=%d error=%v", runID, err)
	}

	var beforeDailyRuns int64
	if err := db.Raw("SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts").Scan(&beforeDailyRuns).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restoreConcurrentProjectionEvidence(t, db, runID, beforeDailyRuns)
	})
	if err := db.Exec("DELETE FROM ai_run_dashboard_facts WHERE run_id = ?", runID).Error; err != nil {
		t.Fatal(err)
	}

	const workers = 8
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			errorsByWorker <- db.Transaction(func(tx *gorm.DB) error {
				return ProjectTerminalDashboardFacts(context.Background(), tx, runID)
			})
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent projection failed: %v", err)
		}
	}

	var factCount, afterDailyRuns int64
	if err := db.Raw("SELECT COUNT(*) FROM ai_run_dashboard_facts WHERE run_id = ?", runID).Scan(&factCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts").Scan(&afterDailyRuns).Error; err != nil {
		t.Fatal(err)
	}
	if factCount != 1 || afterDailyRuns != beforeDailyRuns+1 {
		t.Fatalf("concurrent projection fact_count=%d daily_runs=%d want fact_count=1 daily_runs=%d", factCount, afterDailyRuns, beforeDailyRuns+1)
	}
}

func openDashboardProjectionEvidenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	if strings.TrimSpace(os.Getenv("AI_RUN_DASHBOARD_PERF")) != "1" {
		t.Skip("AI_RUN_DASHBOARD_PERF=1 is required for dashboard projection evidence")
	}
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	configuration, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}
	if configuration.DBName != "admin_ai_dashboard_perf" {
		t.Fatalf("TEST_MYSQL_DSN must target admin_ai_dashboard_perf, got %q", configuration.DBName)
	}
	configuration.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(configuration.FormatDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open dashboard projection database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func restoreConcurrentProjectionEvidence(t *testing.T, db *gorm.DB, runID, beforeDailyRuns int64) {
	t.Helper()
	var factCount int64
	if err := db.Raw("SELECT COUNT(*) FROM ai_run_dashboard_facts WHERE run_id = ?", runID).Scan(&factCount).Error; err != nil {
		t.Errorf("query concurrent projection cleanup fact: %v", err)
		return
	}
	if factCount == 0 {
		if err := db.Transaction(func(tx *gorm.DB) error {
			return ProjectTerminalDashboardFacts(context.Background(), tx, runID)
		}); err != nil {
			t.Errorf("restore concurrent projection fact: %v", err)
			return
		}
	}

	const decrementDailySQL = `
UPDATE ai_run_dashboard_daily_facts d
JOIN ai_run_dashboard_facts f
  ON d.fact_date = f.fact_date AND d.platform = f.platform AND d.model_id = f.model_id
  AND d.agent_id = f.agent_id AND d.provider_id = f.provider_id AND d.user_id = f.user_id
  AND d.status = f.status AND d.run_anomaly_code = f.run_anomaly_code
  AND d.billing_anomaly_code = f.billing_anomaly_code AND d.final_error_code = f.final_error_code
SET d.run_count = d.run_count - 1,
    d.prompt_tokens = d.prompt_tokens - f.prompt_tokens,
    d.completion_tokens = d.completion_tokens - f.completion_tokens,
    d.total_tokens = d.total_tokens - f.total_tokens,
    d.settled_runs = d.settled_runs - f.settled_runs,
    d.actual_units = d.actual_units - f.actual_units,
    d.released_runs = d.released_runs - f.released_runs,
    d.released_units = d.released_units - f.released_units,
    d.unbilled_runs = d.unbilled_runs - f.unbilled_runs
WHERE f.run_id = ?`
	var currentDailyRuns int64
	if err := db.Raw("SELECT COALESCE(SUM(run_count), 0) FROM ai_run_dashboard_daily_facts").Scan(&currentDailyRuns).Error; err != nil {
		t.Errorf("query concurrent projection cleanup aggregate: %v", err)
		return
	}
	for currentDailyRuns > beforeDailyRuns {
		if err := db.Exec(decrementDailySQL, runID).Error; err != nil {
			t.Errorf("restore concurrent projection aggregate: %v", err)
			return
		}
		currentDailyRuns--
	}
	if currentDailyRuns != beforeDailyRuns {
		t.Errorf("concurrent projection cleanup daily_runs=%d want %d", currentDailyRuns, beforeDailyRuns)
	}
}
