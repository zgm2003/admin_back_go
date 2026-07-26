package aitext

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCanonicalRunLookupPrecedesTextTaskAcceptance(t *testing.T) {
	db := dryRunTextDB(t)
	var run canonicalRunRow
	statement := canonicalRunLookupDB(db, 7, "request-cross-entry").Take(&run).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	if !strings.Contains(query, "FROM `ai_runs`") || !strings.Contains(query, "user_id = ? AND request_id = ?") {
		t.Fatalf("canonical lookup does not own request identity: %s", query)
	}
	if strings.Contains(query, "ai_text_tasks") {
		t.Fatalf("canonical request identity incorrectly depends on text task: %s", query)
	}
}

func TestPendingTaskLookupFindsAcceptedRunningWorkWithoutQueueEvidence(t *testing.T) {
	db := dryRunTextDB(t)
	var rows []TextTask
	statement := pendingTasksDB(db, 25).Find(&rows).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	for _, required := range []string{"FROM ai_text_tasks AS t", "JOIN ai_runs r ON r.id = t.run_id", "t.status = ?", "r.status = ?", "ORDER BY t.created_at ASC, t.id ASC", "LIMIT ?"} {
		if !strings.Contains(query, required) {
			t.Fatalf("pending task query missing %q: %s", required, query)
		}
	}
}

type pendingOnlyStore struct{ task *TextTask }

func (s pendingOnlyStore) Accept(context.Context, AcceptInput) (*TextTask, error) {
	return nil, ErrStoreNotConfigured
}
func (s pendingOnlyStore) FindByID(context.Context, uint64) (*TextTask, error) {
	return nil, ErrStoreNotConfigured
}
func (s pendingOnlyStore) FindReplay(context.Context, int64, string) (*ReplayRecord, error) {
	return nil, ErrStoreNotConfigured
}
func (s pendingOnlyStore) FindPending(context.Context, int) ([]TextTask, error) {
	if s.task == nil {
		return nil, nil
	}
	return []TextTask{*cloneTask(s.task)}, nil
}

func TestReconcilerWakesAcceptedTaskAfterSubmitEnqueueCrashWindow(t *testing.T) {
	fingerprint := sha256.Sum256([]byte("request"))
	waker := &fakeWaker{}
	reconciler := NewReconciler(pendingOnlyStore{task: &TextTask{ID: 41, RunID: 51, Status: StatusRunning, RequestFingerprint: fingerprint[:]}}, waker, 10)

	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked || waker.calls != 1 || waker.taskID != 41 {
		t.Fatalf("reconcile worked=%v err=%v waker=%+v", worked, err, waker)
	}
}

func TestReplayLookupUsesOnlyDurableTaskAndRunFacts(t *testing.T) {
	db := dryRunTextDB(t)
	var row replayRow
	statement := replayLookupDB(db, 7, "request-replay").Take(&row).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	for _, required := range []string{
		"FROM ai_text_tasks AS t",
		"JOIN ai_runs r ON r.id = t.run_id AND r.user_id = t.user_id AND r.request_id = t.request_id",
		"t.user_id = ? AND t.request_id = ?",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("replay query missing %q: %s", required, query)
		}
	}
	for _, mutableTable := range []string{"ai_agents", "ai_providers"} {
		if strings.Contains(query, mutableTable) {
			t.Fatalf("replay query depends on mutable %s: %s", mutableTable, query)
		}
	}
}

func dryRunTextDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("mysql", "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}
	return db
}
