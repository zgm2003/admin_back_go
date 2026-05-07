package crontask

import (
	"strings"

	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payruntime"
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
		Name:        "notification_task_scheduler",
		TaskType:    notificationtask.TypeDispatchDueV1,
		Description: "扫描待发送通知任务并投递 notification send-task 队列任务",
		BuildTask: func() (taskqueue.Task, error) {
			return notificationtask.NewDispatchDueTask(notificationtask.DispatchDuePayload{})
		},
	})
	registry.Register(RegistryEntry{
		Name:        "pay_close_expired_order",
		TaskType:    payruntime.TypeCloseExpiredOrderV1,
		Description: "扫描过期支付宝充值订单，先查单再自动关闭或入账",
		BuildTask: func() (taskqueue.Task, error) {
			return payruntime.NewCloseExpiredOrderTask(payruntime.CloseExpiredOrderPayload{})
		},
	})
	registry.Register(RegistryEntry{
		Name:        "pay_sync_pending_transaction",
		TaskType:    payruntime.TypeSyncPendingTransactionV1,
		Description: "扫描待补查支付宝流水，主动查单并补偿支付成功",
		BuildTask: func() (taskqueue.Task, error) {
			return payruntime.NewSyncPendingTransactionTask(payruntime.SyncPendingTransactionPayload{})
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
