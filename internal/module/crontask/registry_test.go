package crontask

import (
	"testing"

	"admin_back_go/internal/module/notificationtask"
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

	payEntry, ok := registry.Lookup("pay_close_expired_order")
	if !ok {
		t.Fatalf("expected pay_close_expired_order registry entry")
	}
	if payEntry.TaskType != payruntime.TypeCloseExpiredOrderV1 {
		t.Fatalf("expected pay task type %s, got %s", payruntime.TypeCloseExpiredOrderV1, payEntry.TaskType)
	}
}
