package crontask

import (
	"testing"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payment"
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

	payEntry, ok := registry.Lookup("payment_close_expired_order")
	if !ok {
		t.Fatalf("expected payment_close_expired_order registry entry")
	}
	if payEntry.TaskType != payment.TypeCloseExpiredOrderV1 {
		t.Fatalf("expected payment task type %s, got %s", payment.TypeCloseExpiredOrderV1, payEntry.TaskType)
	}
}

func TestDefaultRegistryMapsPaymentCronTasksToVersionedTaskTypes(t *testing.T) {
	registry := NewDefaultRegistry()

	closeEntry, ok := registry.Lookup("payment_close_expired_order")
	if !ok {
		t.Fatalf("expected payment_close_expired_order to be registered")
	}
	if closeEntry.TaskType != payment.TypeCloseExpiredOrderV1 {
		t.Fatalf("unexpected close task type: %s", closeEntry.TaskType)
	}
	if closeEntry.BuildTask == nil {
		t.Fatalf("expected close BuildTask")
	}
	closeTask, err := closeEntry.BuildTask()
	if err != nil {
		t.Fatalf("close BuildTask returned error: %v", err)
	}
	if closeTask.Type != payment.TypeCloseExpiredOrderV1 {
		t.Fatalf("unexpected built close task type: %s", closeTask.Type)
	}

	syncEntry, ok := registry.Lookup("payment_sync_pending_order")
	if !ok {
		t.Fatalf("expected payment_sync_pending_order to be registered")
	}
	if syncEntry.TaskType != payment.TypeSyncPendingOrderV1 {
		t.Fatalf("unexpected sync task type: %s", syncEntry.TaskType)
	}
	if syncEntry.BuildTask == nil {
		t.Fatalf("expected sync BuildTask")
	}
	syncTask, err := syncEntry.BuildTask()
	if err != nil {
		t.Fatalf("sync BuildTask returned error: %v", err)
	}
	if syncTask.Type != payment.TypeSyncPendingOrderV1 {
		t.Fatalf("unexpected built sync task type: %s", syncTask.Type)
	}
}

func TestDefaultRegistryDoesNotKeepOldPayCronNames(t *testing.T) {
	registry := NewDefaultRegistry()
	oldNames := []string{
		"pay_close_expired_order",
		"pay_sync_pending_transaction",
		"pay_fulfillment_retry",
		"pay_reconcile_daily",
		"pay_reconcile_execute",
	}
	for _, name := range oldNames {
		if entry, ok := registry.Lookup(name); ok {
			t.Fatalf("old pay cron %s must not stay registered: %#v", name, entry)
		}
	}
}
