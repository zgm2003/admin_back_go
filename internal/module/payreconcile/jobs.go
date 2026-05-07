package payreconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	TypeReconcileDailyV1   = "pay:reconcile-daily:v1"
	TypeReconcileExecuteV1 = "pay:reconcile-execute:v1"
)

type ReconcileDailyPayload struct {
	Date  string `json:"date,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type ReconcileExecutePayload struct {
	TaskID int64 `json:"task_id,omitempty"`
	Limit  int   `json:"limit,omitempty"`
}

type JobService interface {
	CreateDailyTasks(ctx context.Context, input CreateDailyTasksInput) (*CreateDailyTasksResult, error)
	ExecutePendingTasks(ctx context.Context, input ExecutePendingTasksInput) (*ExecutePendingTasksResult, error)
	ExecuteTask(ctx context.Context, taskID int64) (*ExecuteTaskResult, error)
}

func NewReconcileDailyTask(payload ReconcileDailyPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeReconcileDailyV1, err)
	}
	return taskqueue.Task{Type: TypeReconcileDailyV1, Payload: data, Queue: taskqueue.QueueLow, UniqueTTL: 55 * time.Second}, nil
}

func NewReconcileExecuteTask(payload ReconcileExecutePayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeReconcileExecuteV1, err)
	}
	return taskqueue.Task{Type: TypeReconcileExecuteV1, Payload: data, Queue: taskqueue.QueueLow, UniqueTTL: 4*time.Minute + 55*time.Second}, nil
}

func DecodeReconcileDailyPayload(payload []byte) (ReconcileDailyPayload, error) {
	var row ReconcileDailyPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return ReconcileDailyPayload{}, fmt.Errorf("decode %s payload: %w", TypeReconcileDailyV1, err)
	}
	return row, nil
}

func DecodeReconcileExecutePayload(payload []byte) (ReconcileExecutePayload, error) {
	var row ReconcileExecutePayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return ReconcileExecutePayload{}, fmt.Errorf("decode %s payload: %w", TypeReconcileExecuteV1, err)
	}
	return row, nil
}

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	if mux == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	mux.HandleFunc(TypeReconcileDailyV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeReconcileDailyPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.CreateDailyTasks(ctx, CreateDailyTasksInput{Date: payload.Date, Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed pay reconcile daily task", "date", result.Date, "scanned", result.Scanned, "created", result.Created, "existing", result.Existing, "skipped", result.Skipped)
		return nil
	})

	mux.HandleFunc(TypeReconcileExecuteV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeReconcileExecutePayload(task.Payload)
		if err != nil {
			return err
		}
		if payload.TaskID > 0 {
			result, err := service.ExecuteTask(ctx, payload.TaskID)
			if err != nil {
				return err
			}
			logger.InfoContext(ctx, "processed pay reconcile execute task", "task_id", result.TaskID, "status", result.Status, "diff_count", result.DiffCount)
			return nil
		}
		result, err := service.ExecutePendingTasks(ctx, ExecutePendingTasksInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed pay reconcile execute pending task", "scanned", result.Scanned, "success", result.Success, "diff", result.Diff, "failed", result.Failed, "skipped", result.Skipped)
		return nil
	})
}
