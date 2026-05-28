package crontask

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"admin_back_go/internal/infra/scheduler"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payment"
)

func TestSchedulerServiceRegistersOnlyEnabledRegisteredTasks(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{tasks: []Task{
		{ID: 1, Name: "notification_task_scheduler", Title: "通知任务调度器", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
		{ID: 2, Name: "payment_close_expired_order", Title: "支付超时关单", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
		{ID: 3, Name: "bad_cron", Title: "错误表达式", Cron: "bad", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
	}}
	registrar := &fakeScheduleRegistrar{}
	enqueuer := &fakeEnqueuer{}
	service := NewSchedulerService(repo, NewDefaultRegistry(), enqueuer, slog.Default())

	if err := service.RegisterEnabled(context.Background(), registrar); err != nil {
		t.Fatalf("RegisterEnabled returned error: %v", err)
	}
	if len(registrar.cronCalls) != 2 {
		t.Fatalf("expected two cron registrations, got %#v", registrar.cronCalls)
	}
	registered := map[string]bool{}
	for _, call := range registrar.cronCalls {
		registered[call.name] = true
	}
	if !registered["notification_task_scheduler"] || !registered["payment_close_expired_order"] {
		t.Fatalf("unexpected registered jobs: %#v", registrar.cronCalls)
	}
}

func TestSchedulerTaskLogsAndEnqueues(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{tasks: []Task{{ID: 1, Name: "notification_task_scheduler", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo}}}
	registrar := &fakeScheduleRegistrar{}
	enqueuer := &fakeEnqueuer{}
	service := NewSchedulerService(repo, NewDefaultRegistry(), enqueuer, slog.Default())
	service.now = func() time.Time { return now }

	if err := service.RegisterEnabled(context.Background(), registrar); err != nil {
		t.Fatalf("RegisterEnabled returned error: %v", err)
	}
	if err := registrar.cronCalls[0].task(context.Background()); err != nil {
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
	for name, taskType := range map[string]string{"payment_close_expired_order": payment.TypeCloseExpiredOrderV1, "payment_sync_pending_order": payment.TypeSyncPendingOrderV1} {
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
	repo := &fakeRepository{tasks: []Task{{ID: 1, Name: "notification_task_scheduler", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo}}}
	registrar := &fakeScheduleRegistrar{}
	enqueuer := &fakeEnqueuer{err: errors.New("redis down")}
	service := NewSchedulerService(repo, NewDefaultRegistry(), enqueuer, slog.Default())

	if err := service.RegisterEnabled(context.Background(), registrar); err != nil {
		t.Fatalf("RegisterEnabled returned error: %v", err)
	}
	if err := registrar.cronCalls[0].task(context.Background()); err == nil {
		t.Fatalf("expected enqueue error")
	}
	if len(repo.endedLogs) != 1 || repo.endedLogs[0].success {
		t.Fatalf("expected failed scheduler log, got %#v", repo.endedLogs)
	}
}

type fakeScheduleRegistrar struct {
	cronCalls []registeredCronCall
}

type registeredCronCall struct {
	name        string
	expression  string
	withSeconds bool
	task        scheduler.TaskFunc
}

func (f *fakeScheduleRegistrar) Every(name string, interval time.Duration, task scheduler.TaskFunc) error {
	return nil
}

func (f *fakeScheduleRegistrar) Cron(name string, expression string, withSeconds bool, task scheduler.TaskFunc) error {
	f.cronCalls = append(f.cronCalls, registeredCronCall{name: name, expression: expression, withSeconds: withSeconds, task: task})
	return nil
}

type fakeEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	if f.err != nil {
		return taskqueue.EnqueueResult{}, f.err
	}
	f.tasks = append(f.tasks, task)
	return taskqueue.EnqueueResult{ID: "task-id", Type: task.Type, Queue: task.Queue}, nil
}
