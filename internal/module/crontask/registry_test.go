package crontask

import (
	"testing"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payment"
)

func TestDefaultRegistryContainsCoreSchedulers(t *testing.T) {
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

	paymentEntries := map[string]string{
		"payment_sync_pending_order":  payment.TypeSyncPendingOrderV1,
		"payment_close_expired_order": payment.TypeCloseExpiredOrderV1,
	}
	for name, taskType := range paymentEntries {
		entry, ok := registry.Lookup(name)
		if !ok {
			t.Fatalf("expected payment registry entry %s", name)
		}
		if entry.TaskType != taskType {
			t.Fatalf("expected payment task type %s, got %#v", taskType, entry)
		}
		task, err := entry.BuildTask()
		if err != nil {
			t.Fatalf("payment BuildTask %s returned error: %v", name, err)
		}
		if task.Type != taskType {
			t.Fatalf("expected built payment task type %s, got %s", taskType, task.Type)
		}
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
