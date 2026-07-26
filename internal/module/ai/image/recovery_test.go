package aiimage

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCanonicalImageRunLookupOwnsRequestIdentityAcrossEntrypoints(t *testing.T) {
	db := dryRunImageDB(t)
	var run canonicalImageRunRow
	statement := canonicalImageRunLookupDB(db, 7, "request-cross-entry").Take(&run).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	if !strings.Contains(query, "FROM `ai_runs`") || !strings.Contains(query, "user_id = ? AND request_id = ?") {
		t.Fatalf("canonical image lookup does not own request identity: %s", query)
	}
	if strings.Contains(query, "ai_image_tasks") {
		t.Fatalf("canonical image identity incorrectly depends on image task: %s", query)
	}
}

func TestPendingImageLookupFindsAcceptedWorkWithoutQueueEvidence(t *testing.T) {
	db := dryRunImageDB(t)
	var tasks []ImageTask
	statement := pendingImageTasksDB(db, 25).Find(&tasks).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	for _, required := range []string{"FROM ai_image_tasks AS t", "JOIN ai_runs r ON r.id = t.run_id", "t.status = ?", "t.lease_expires_at IS NULL OR t.lease_expires_at <= ?", "r.status = ?", "ORDER BY t.created_at ASC, t.id ASC", "LIMIT ?"} {
		if !strings.Contains(query, required) {
			t.Fatalf("pending image query missing %q: %s", required, query)
		}
	}
	if strings.Contains(query, "t.is_del") {
		t.Fatalf("pending image recovery must include user-hidden accepted work: %s", query)
	}
	if strings.Contains(query, "dispatched_at") {
		t.Fatalf("pending image recovery must use the renewable task lease, not a fixed attempt timeout: %s", query)
	}
}

func TestImageLeaseClaimAllowsPendingOrExpiredRunningWorkWithoutVisibilityFilter(t *testing.T) {
	db := dryRunImageDB(t)
	var task ImageTask
	statement := claimableImageTaskDB(db, "admin", 7, 11, time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)).Take(&task).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")
	for _, required := range []string{"status = ?", "lease_expires_at IS NULL", "lease_expires_at <= ?", "platform = ?", "user_id = ?", "id = ?"} {
		if !strings.Contains(query, required) {
			t.Fatalf("claimable image query missing %q: %s", required, query)
		}
	}
	if strings.Contains(query, "is_del") {
		t.Fatalf("lease claim must continue user-hidden accepted work: %s", query)
	}
}

func TestWorkerImageTaskLookupIncludesUserHiddenAcceptedWork(t *testing.T) {
	db := dryRunImageDB(t)
	var task ImageTask
	statement := workerTaskDB(db, "admin", 7, 11).Take(&task).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	for _, required := range []string{"FROM `ai_image_tasks`", "platform = ?", "user_id = ?", "id = ?"} {
		if !strings.Contains(query, required) {
			t.Fatalf("worker image task query missing %q: %s", required, query)
		}
	}
	if strings.Contains(query, "is_del") {
		t.Fatalf("worker image task lookup must include user-hidden accepted work: %s", query)
	}
}

func TestUserImageTaskLookupExcludesSoftDeletedWork(t *testing.T) {
	db := dryRunImageDB(t)
	var task ImageTask
	statement := userTaskDB(db, "admin", 7, 11).Take(&task).Statement
	query := strings.Join(strings.Fields(statement.SQL.String()), " ")

	if !strings.Contains(query, "is_del = ?") {
		t.Fatalf("user image task lookup must exclude soft-deleted work: %s", query)
	}
}

type fakeImagePendingStore struct{ task *ImageTask }

func (store fakeImagePendingStore) FindPendingImages(context.Context, int) ([]ImageTask, error) {
	if store.task == nil {
		return nil, nil
	}
	return []ImageTask{*cloneImageTask(store.task)}, nil
}

type fakeImageTaskWaker struct {
	calls int
	task  ImageTask
}

func (waker *fakeImageTaskWaker) WakeImageTask(_ context.Context, task ImageTask) error {
	waker.calls++
	waker.task = task
	return nil
}

func TestImageReconcilerWakesAcceptedTaskAfterSubmitEnqueueCrashWindow(t *testing.T) {
	task := &ImageTask{ID: 41, RunID: 51, UserID: 7, Platform: "admin", Status: StatusPending}
	waker := &fakeImageTaskWaker{}
	reconciler := NewReconciler(fakeImagePendingStore{task: task}, waker, 10)

	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked || waker.calls != 1 || waker.task.ID != task.ID {
		t.Fatalf("reconcile worked=%v err=%v waker=%+v", worked, err, waker)
	}
}

func dryRunImageDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("mysql", "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}
	return db
}
