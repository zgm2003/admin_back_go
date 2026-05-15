package crontask

import (
	"testing"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/notificationtask"
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

	for _, name := range []string{"payment_close_expired_order", "payment_sync_pending_order"} {
		if entry, ok := registry.Lookup(name); ok {
			t.Fatalf("payment order cron must be absent until an automatic close/sync slice exists: %s %#v", name, entry)
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
