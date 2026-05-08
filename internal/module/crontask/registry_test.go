package crontask

import (
	"testing"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payreconcile"
	"admin_back_go/internal/module/payruntime"
)

func TestDefaultRegistryContainsNotificationTaskSchedulerOnly(t *testing.T) {
	registry := NewDefaultRegistry()

	entry, ok := registry.Lookup("notification_task_scheduler")
	if !ok {
		t.Fatalf("expected notification_task_scheduler registry entry")
	}
	if entry.TaskType != notificationtask.TypeDispatchDueV1 {
		t.Fatalf("expected task type %s, got %s", notificationtask.TypeDispatchDueV1, entry.TaskType)
	}
	if entry.BuildTask == nil {
		t.Fatalf("expected BuildTask")
	}

	task, err := entry.BuildTask()
	if err != nil {
		t.Fatalf("BuildTask returned error: %v", err)
	}
	if task.Type != notificationtask.TypeDispatchDueV1 {
		t.Fatalf("expected task type %s, got %s", notificationtask.TypeDispatchDueV1, task.Type)
	}

	aiEntry, ok := registry.Lookup("ai_run_timeout")
	if !ok {
		t.Fatalf("expected ai_run_timeout registry entry")
	}
	if aiEntry.TaskType != aichat.TypeRunTimeoutV1 {
		t.Fatalf("expected ai task type %s, got %s", aichat.TypeRunTimeoutV1, aiEntry.TaskType)
	}
	aiTask, err := aiEntry.BuildTask()
	if err != nil {
		t.Fatalf("ai BuildTask returned error: %v", err)
	}
	if aiTask.Type != aichat.TypeRunTimeoutV1 {
		t.Fatalf("expected built ai task type %s, got %s", aichat.TypeRunTimeoutV1, aiTask.Type)
	}

	payEntry, ok := registry.Lookup("pay_close_expired_order")
	if !ok {
		t.Fatalf("expected pay_close_expired_order registry entry")
	}
	if payEntry.TaskType != payruntime.TypeCloseExpiredOrderV1 {
		t.Fatalf("expected pay task type %s, got %s", payruntime.TypeCloseExpiredOrderV1, payEntry.TaskType)
	}

	retryEntry, ok := registry.Lookup("pay_fulfillment_retry")
	if !ok {
		t.Fatalf("expected pay_fulfillment_retry registry entry")
	}
	if retryEntry.TaskType != payruntime.TypeFulfillmentRetryV1 {
		t.Fatalf("expected pay fulfillment retry type %s, got %s", payruntime.TypeFulfillmentRetryV1, retryEntry.TaskType)
	}
}

func TestDefaultRegistryMapsPayReconcileCronTasksToVersionedTaskTypes(t *testing.T) {
	registry := NewDefaultRegistry()

	dailyEntry, ok := registry.Lookup("pay_reconcile_daily")
	if !ok {
		t.Fatalf("expected pay_reconcile_daily to be registered")
	}
	if dailyEntry.TaskType != payreconcile.TypeReconcileDailyV1 {
		t.Fatalf("unexpected daily task type: %s", dailyEntry.TaskType)
	}
	if dailyEntry.BuildTask == nil {
		t.Fatalf("expected daily BuildTask")
	}
	dailyTask, err := dailyEntry.BuildTask()
	if err != nil {
		t.Fatalf("daily BuildTask returned error: %v", err)
	}
	if dailyTask.Type != payreconcile.TypeReconcileDailyV1 {
		t.Fatalf("unexpected built daily task type: %s", dailyTask.Type)
	}

	executeEntry, ok := registry.Lookup("pay_reconcile_execute")
	if !ok {
		t.Fatalf("expected pay_reconcile_execute to be registered")
	}
	if executeEntry.TaskType != payreconcile.TypeReconcileExecuteV1 {
		t.Fatalf("unexpected execute task type: %s", executeEntry.TaskType)
	}
	if executeEntry.BuildTask == nil {
		t.Fatalf("expected execute BuildTask")
	}
	executeTask, err := executeEntry.BuildTask()
	if err != nil {
		t.Fatalf("execute BuildTask returned error: %v", err)
	}
	if executeTask.Type != payreconcile.TypeReconcileExecuteV1 {
		t.Fatalf("unexpected built execute task type: %s", executeTask.Type)
	}
}
