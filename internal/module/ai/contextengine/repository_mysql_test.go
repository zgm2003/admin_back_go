package contextengine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPersistTerminalMySQLConcurrentLoserReloadsWinner(t *testing.T) {
	repository, sqlDB := openPlanRepositoryIntegrationDB(t)
	runID := uint64(time.Now().UnixNano())
	defer deletePlanIntegrationFixture(t, sqlDB, runID)

	left := validReadyPlan()
	left.RunID = runID
	right := left
	right.InputFingerprintSHA256 = hashText("mysql-right-input")
	rightHash := hashText("mysql-right-plan")
	right.PlanSHA256 = &rightHash

	guard := newPlanCommitBarrier(2)
	results := make(chan ContextPlan, 2)
	dispositions := make(chan PersistDisposition, 2)
	errorsChannel := make(chan error, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var workers sync.WaitGroup
	for _, candidate := range []ContextPlan{left, right} {
		candidate := candidate
		workers.Add(1)
		go func() {
			defer workers.Done()
			plan, disposition, err := repository.PersistTerminal(ctx, candidate, guard, validPlanCommitToken(candidate))
			results <- plan
			dispositions <- disposition
			errorsChannel <- err
		}()
	}
	guard.WaitAndRelease(t, ctx)
	workers.Wait()
	close(results)
	close(dispositions)
	close(errorsChannel)

	var plans []ContextPlan
	for plan := range results {
		plans = append(plans, plan)
	}
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[PersistDisposition]int{}
	for disposition := range dispositions {
		seen[disposition]++
	}
	if seen[PersistCreated] != 1 || seen[PersistLoadedExisting] != 1 {
		t.Fatalf("persist dispositions = %#v", seen)
	}
	if len(plans) != 2 || plans[0].ID == 0 || plans[0].ID != plans[1].ID || plans[0].PlanSHA256 == nil ||
		plans[1].PlanSHA256 == nil || *plans[0].PlanSHA256 != *plans[1].PlanSHA256 {
		t.Fatalf("concurrent callers returned different terminal plans: %#v", plans)
	}
}

func TestPersistTerminalMySQLRoundTripsDegradedPlan(t *testing.T) {
	repository, sqlDB := openPlanRepositoryIntegrationDB(t)
	runID := uint64(time.Now().UnixNano())
	defer deletePlanIntegrationFixture(t, sqlDB, runID)

	candidate := degradedReadyPlan(t)
	candidate.RunID = runID
	got, disposition, err := repository.PersistTerminal(t.Context(), candidate, &recordingPlanGuard{}, validPlanCommitToken(candidate))
	if err != nil {
		t.Fatal(err)
	}
	if disposition != PersistCreated || got.ID == 0 || got.State != PlanReady || got.RetrievalOutcome != RetrievalDegraded || got.PlanSHA256 == nil {
		t.Fatalf("persisted degraded plan = %#v, disposition = %q", got, disposition)
	}
	loaded, err := repository.FindTerminalByRunID(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Error == nil || loaded.Error.Stage != candidate.Error.Stage || loaded.Error.Code != candidate.Error.Code {
		t.Fatalf("loaded degraded plan = %#v", loaded)
	}
}

type planCommitBarrier struct {
	arrived chan struct{}
	release chan struct{}
	count   int
}

func newPlanCommitBarrier(count int) *planCommitBarrier {
	return &planCommitBarrier{arrived: make(chan struct{}, count), release: make(chan struct{}), count: count}
}

func (guard *planCommitBarrier) GuardPlanCommitInTransaction(
	_ context.Context,
	_ *gorm.DB,
	_ PlanCommitToken,
) (PlanCommitGuardResult, error) {
	guard.arrived <- struct{}{}
	<-guard.release
	return PlanCommitGuardResult{}, nil
}

func (guard *planCommitBarrier) WaitAndRelease(t *testing.T, ctx context.Context) {
	t.Helper()
	for range guard.count {
		select {
		case <-guard.arrived:
		case <-ctx.Done():
			t.Fatalf("wait for concurrent plan transactions: %v", ctx.Err())
		}
	}
	close(guard.release)
}

func openPlanRepositoryIntegrationDB(t *testing.T) (*GormPlanRepository, *sql.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is required for context plan concurrency integration test")
	}
	configuration, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}
	databaseName := strings.ToLower(strings.TrimSpace(configuration.DBName))
	if databaseName == "" || !strings.Contains(databaseName, "test") || isSystemDatabase(databaseName) {
		t.Fatalf("TEST_MYSQL_DSN must target a dedicated test database, got %q", configuration.DBName)
	}
	if configuration.Params == nil {
		configuration.Params = map[string]string{}
	}
	configuration.Params["foreign_key_checks"] = "0"
	sqlDB, err := sql.Open("mysql", configuration.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(4)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping test MySQL: %v", err)
	}
	for _, table := range []string{"ai_context_plans", "ai_context_plan_items"} {
		var count int
		if err := sqlDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", table,
		).Scan(&count); err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s is missing; apply the Context Expand migration to the dedicated test database", table)
		}
	}

	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return NewPlanRepository(&database.Client{Gorm: gormDB, SQL: sqlDB}), sqlDB
}

func deletePlanIntegrationFixture(t *testing.T, sqlDB *sql.DB, runID uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := sqlDB.ExecContext(ctx,
		"DELETE item FROM ai_context_plan_items AS item JOIN ai_context_plans AS plan ON plan.id = item.plan_id WHERE plan.run_id = ?", runID,
	); err != nil {
		t.Errorf("delete context plan item fixture: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, "DELETE FROM ai_context_plans WHERE run_id = ?", runID); err != nil {
		t.Errorf("delete context plan fixture: %v", err)
	}
}

func isSystemDatabase(name string) bool {
	switch name {
	case "mysql", "information_schema", "performance_schema", "sys":
		return true
	}
	return false
}

func hashText(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}
