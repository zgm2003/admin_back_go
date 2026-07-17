package crontask

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	notificationtask "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/module/payment"
)

func TestSchedulerTaskLogsAndEnqueues(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	row := Task{ID: 1, Name: "notification_task_scheduler", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo}
	repo := &fakeRepository{tasks: []Task{row}}
	enqueuer := &fakeEnqueuer{}
	registry := NewDefaultRegistry()
	service := NewSchedulerService(repo, registry, enqueuer, slog.Default())
	service.now = func() time.Time { return now }
	entry, ok := registry.Lookup(row.Name)
	if !ok {
		t.Fatal("notification task is not registered")
	}

	if err := service.taskFunc(row, entry)(context.Background()); err != nil {
		t.Fatalf("scheduled task returned error: %v", err)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type != notificationtask.TypeDispatchDueV1 {
		t.Fatalf("expected notification dispatch task enqueue, got %#v", enqueuer.tasks)
	}
	if len(repo.startedLogs) != 1 || len(repo.endedLogs) != 1 || !repo.endedLogs[0].success {
		t.Fatalf("expected success scheduler log, start=%#v end=%#v", repo.startedLogs, repo.endedLogs)
	}
}

func TestSchedulerDefaultRegistryIncludesPaymentOrderCronTasks(t *testing.T) {
	registry := NewDefaultRegistry()
	for name, taskType := range map[string]string{
		"payment_close_expired_order": payment.TypeCloseExpiredOrderV1,
		"payment_sync_pending_order":  payment.TypeSyncPendingOrderV1,
	} {
		entry, ok := registry.Lookup(name)
		if !ok || entry.TaskType != taskType {
			t.Fatalf("payment order cron must be registered now: %s %#v", name, entry)
		}
	}
	for _, oldName := range []string{
		"pay_close_expired_order",
		"pay_sync_pending_transaction",
		"pay_fulfillment_retry",
		"pay_reconcile_daily",
		"pay_reconcile_execute",
	} {
		if entry, ok := registry.Lookup(oldName); ok {
			t.Fatalf("old pay cron %s must not stay registered: %#v", oldName, entry)
		}
	}
}

func TestSchedulerTaskWritesFailedLogWhenEnqueueFails(t *testing.T) {
	row := Task{ID: 1, Name: "notification_task_scheduler", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo}
	repo := &fakeRepository{tasks: []Task{row}}
	enqueuer := &fakeEnqueuer{err: errors.New("redis down")}
	registry := NewDefaultRegistry()
	service := NewSchedulerService(repo, registry, enqueuer, slog.Default())
	entry, ok := registry.Lookup(row.Name)
	if !ok {
		t.Fatal("notification task is not registered")
	}

	if err := service.taskFunc(row, entry)(context.Background()); err == nil {
		t.Fatal("expected enqueue error")
	}
	if len(repo.endedLogs) != 1 || repo.endedLogs[0].success {
		t.Fatalf("expected failed scheduler log, got %#v", repo.endedLogs)
	}
}

func TestSchedulerTaskDoesNotEnqueueAfterContextCancellation(t *testing.T) {
	repo := &fakeRepository{}
	enqueuer := &fakeEnqueuer{}
	registry := NewDefaultRegistry()
	service := NewSchedulerService(repo, registry, enqueuer, slog.Default())
	entry, ok := registry.Lookup("notification_task_scheduler")
	if !ok {
		t.Fatal("notification task is not registered")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.taskFunc(Task{ID: 1, Name: "notification_task_scheduler"}, entry)(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if len(enqueuer.tasks) != 0 || len(repo.startedLogs) != 0 {
		t.Fatalf("canceled callback performed side effects: tasks=%#v logs=%#v", enqueuer.tasks, repo.startedLogs)
	}
}

func TestSchedulerTaskFencingCheckPreventsEnqueueWhenLeaseIsLostAfterLogStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &cancelAfterLogStartRepository{cancel: cancel}
	enqueuer := &fakeEnqueuer{}
	registry := NewDefaultRegistry()
	service := NewSchedulerService(repo, registry, enqueuer, slog.Default())
	entry, ok := registry.Lookup("notification_task_scheduler")
	if !ok {
		t.Fatal("notification task is not registered")
	}

	err := service.taskFunc(Task{ID: 1, Name: "notification_task_scheduler"}, entry)(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected lease-loss cancellation, got %v", err)
	}
	if len(enqueuer.tasks) != 0 {
		t.Fatalf("task enqueued after fencing context was canceled: %#v", enqueuer.tasks)
	}
	if !repo.ended {
		t.Fatal("started cron log was not closed after lease loss")
	}
}

type fakeEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

type cancelAfterLogStartRepository struct {
	Repository
	cancel context.CancelFunc
	ended  bool
}

func (r *cancelAfterLogStartRepository) LogStart(context.Context, Task, time.Time) (int64, error) {
	r.cancel()
	return 1, nil
}

func (r *cancelAfterLogStartRepository) LogEnd(context.Context, int64, bool, string, string, time.Time) error {
	r.ended = true
	return nil
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	if f.err != nil {
		return taskqueue.EnqueueResult{}, f.err
	}
	f.tasks = append(f.tasks, task)
	return taskqueue.EnqueueResult{ID: "task-id", Type: task.Type, Queue: task.Queue}, nil
}
