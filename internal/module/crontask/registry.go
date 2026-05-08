package crontask

import (
	"strings"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payment"
	"admin_back_go/internal/platform/taskqueue"
)

type RegistryEntry struct {
	Name        string
	TaskType    string
	Description string
	BuildTask   func() (taskqueue.Task, error)
}

type Registry struct {
	entries map[string]RegistryEntry
}

func NewDefaultRegistry() Registry {
	registry := NewRegistry()
	registry.Register(RegistryEntry{
		Name:        "ai_run_timeout",
		TaskType:    aichat.TypeRunTimeoutV1,
		Description: "标记超时 AI 运行失败",
		BuildTask: func() (taskqueue.Task, error) {
			return aichat.NewRunTimeoutTask(aichat.RunTimeoutPayload{})
		},
	})
	registry.Register(RegistryEntry{
		Name:        "notification_task_scheduler",
		TaskType:    notificationtask.TypeDispatchDueV1,
		Description: "扫描待发送通知任务并投递 notification send-task 队列任务",
		BuildTask: func() (taskqueue.Task, error) {
			return notificationtask.NewDispatchDueTask(notificationtask.DispatchDuePayload{})
		},
	})
	registry.Register(RegistryEntry{
		Name:        "payment_close_expired_order",
		TaskType:    payment.TypeCloseExpiredOrderV1,
		Description: "扫描过期支付宝支付订单并关闭或补记成功",
		BuildTask: func() (taskqueue.Task, error) {
			return payment.NewCloseExpiredOrderTask(payment.CloseExpiredPayload{})
		},
	})
	registry.Register(RegistryEntry{
		Name:        "payment_sync_pending_order",
		TaskType:    payment.TypeSyncPendingOrderV1,
		Description: "扫描支付中订单，主动查单并补偿支付成功",
		BuildTask: func() (taskqueue.Task, error) {
			return payment.NewSyncPendingOrderTask(payment.SyncPendingPayload{})
		},
	})
	return registry
}

func NewRegistry() Registry {
	return Registry{entries: make(map[string]RegistryEntry)}
}

func (r Registry) Register(entry RegistryEntry) {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return
	}
	entry.Name = name
	r.entries[name] = entry
}

func (r Registry) Lookup(name string) (RegistryEntry, bool) {
	if len(r.entries) == 0 {
		return RegistryEntry{}, false
	}
	entry, ok := r.entries[strings.TrimSpace(name)]
	return entry, ok
}
