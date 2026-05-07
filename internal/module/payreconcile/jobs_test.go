package payreconcile

import (
	"context"
	"testing"

	"admin_back_go/internal/platform/taskqueue"
)

func TestNewReconcileDailyTaskEncodesPayload(t *testing.T) {
	task, err := NewReconcileDailyTask(ReconcileDailyPayload{Date: "2026-05-06", Limit: 20})
	if err != nil {
		t.Fatalf("NewReconcileDailyTask returned error: %v", err)
	}
	if task.Type != TypeReconcileDailyV1 {
		t.Fatalf("unexpected type: %s", task.Type)
	}
	if task.Queue != taskqueue.QueueLow {
		t.Fatalf("unexpected queue: %s", task.Queue)
	}
	if task.UniqueTTL <= 0 {
		t.Fatalf("expected unique ttl")
	}
	payload, err := DecodeReconcileDailyPayload(task.Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Date != "2026-05-06" || payload.Limit != 20 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestNewReconcileExecuteTaskEncodesPayload(t *testing.T) {
	task, err := NewReconcileExecuteTask(ReconcileExecutePayload{TaskID: 9, Limit: 3})
	if err != nil {
		t.Fatalf("NewReconcileExecuteTask returned error: %v", err)
	}
	if task.Type != TypeReconcileExecuteV1 {
		t.Fatalf("unexpected type: %s", task.Type)
	}
	if task.Queue != taskqueue.QueueLow {
		t.Fatalf("unexpected queue: %s", task.Queue)
	}
	if task.UniqueTTL <= 0 {
		t.Fatalf("expected unique ttl")
	}
	payload, err := DecodeReconcileExecutePayload(task.Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.TaskID != 9 || payload.Limit != 3 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRegisterHandlersWiresReconcileTasks(t *testing.T) {
	mux := taskqueue.NewMux()
	service := &fakeReconcileJobService{}
	RegisterHandlers(mux, service, nil)

	dailyTask, err := NewReconcileDailyTask(ReconcileDailyPayload{Date: "2026-05-06", Limit: 2})
	if err != nil {
		t.Fatalf("NewReconcileDailyTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), dailyTask); err != nil {
		t.Fatalf("daily handler returned error: %v", err)
	}

	executeTask, err := NewReconcileExecuteTask(ReconcileExecutePayload{TaskID: 9, Limit: 3})
	if err != nil {
		t.Fatalf("NewReconcileExecuteTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), executeTask); err != nil {
		t.Fatalf("execute handler returned error: %v", err)
	}

	batchTask, err := NewReconcileExecuteTask(ReconcileExecutePayload{Limit: 4})
	if err != nil {
		t.Fatalf("NewReconcileExecuteTask batch returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), batchTask); err != nil {
		t.Fatalf("execute batch handler returned error: %v", err)
	}

	if service.dailyDate != "2026-05-06" || service.dailyLimit != 2 || service.executeTaskID != 9 || service.executeLimit != 4 {
		t.Fatalf("unexpected service calls: %#v", service)
	}
}

type fakeReconcileJobService struct {
	dailyDate     string
	dailyLimit    int
	executeTaskID int64
	executeLimit  int
}

func (f *fakeReconcileJobService) CreateDailyTasks(ctx context.Context, input CreateDailyTasksInput) (*CreateDailyTasksResult, error) {
	f.dailyDate = input.Date
	f.dailyLimit = input.Limit
	return &CreateDailyTasksResult{}, nil
}

func (f *fakeReconcileJobService) ExecutePendingTasks(ctx context.Context, input ExecutePendingTasksInput) (*ExecutePendingTasksResult, error) {
	f.executeLimit = input.Limit
	return &ExecutePendingTasksResult{}, nil
}

func (f *fakeReconcileJobService) ExecuteTask(ctx context.Context, taskID int64) (*ExecuteTaskResult, error) {
	f.executeTaskID = taskID
	return &ExecuteTaskResult{TaskID: taskID}, nil
}
