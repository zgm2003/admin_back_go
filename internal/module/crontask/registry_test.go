package crontask

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aichat "admin_back_go/internal/module/ai/chat"
	exporttask "admin_back_go/internal/module/export"
	notificationtask "admin_back_go/internal/module/notification/task"
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

	exportEntry, ok := registry.Lookup("export_cleanup_expired")
	if !ok || exportEntry.TaskType != exporttask.TypeCleanupExpiredV1 || exportEntry.BuildTask == nil {
		t.Fatalf("expected export cleanup registry entry, got %#v found=%v", exportEntry, ok)
	}
	exportTask, err := exportEntry.BuildTask()
	if err != nil || exportTask.Type != exporttask.TypeCleanupExpiredV1 {
		t.Fatalf("unexpected export cleanup task=%#v err=%v", exportTask, err)
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

func TestCleanupMigrationSoftDeletesContactRequestCron(t *testing.T) {
	path := filepath.Join("..", "..", "..", "database", "legacy-migrations", "20260521_cron_task_active_cleanup.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected cleanup migration %s: %v", path, err)
	}
	source := string(data)
	for _, want := range []string{
		"clean_expired_contact_request",
		"status` = 2",
		"is_del` = 1",
		"ai_run_timeout",
		"ai:run-timeout:v1",
		"notification_task_scheduler",
		"notification:dispatch-due:v1",
		"payment_sync_pending_order",
		"payment:sync-pending-order:v1",
		"payment_close_expired_order",
		"payment:close-expired-order:v1",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("cleanup migration must contain %q, got:\n%s", want, source)
		}
	}
}
