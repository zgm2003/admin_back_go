package notificationtask

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	TypeDispatchDueV1 = "notification:dispatch-due:v1"
	TypeSendTaskV1    = "notification:send-task:v1"

	ScheduleDispatchDueName     = "notification-task-dispatch-due"
	ScheduleDispatchDueInterval = time.Minute
)

type DispatchDuePayload struct {
	Limit int `json:"limit,omitempty"`
}

type SendTaskPayload struct {
	TaskID int64 `json:"task_id"`
}

type JobService interface {
	DispatchDue(ctx context.Context, input DispatchDueInput) (*DispatchDueResult, error)
	SendTask(ctx context.Context, input SendTaskInput) (*SendTaskResult, error)
}

func NewDispatchDueTask(payload DispatchDuePayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeDispatchDueV1, err)
	}
	return taskqueue.Task{
		Type:      TypeDispatchDueV1,
		Payload:   data,
		Queue:     taskqueue.QueueDefault,
		UniqueTTL: 55 * time.Second,
	}, nil
}

func NewSendTask(taskID int64) (taskqueue.Task, error) {
	data, err := json.Marshal(SendTaskPayload{TaskID: taskID})
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeSendTaskV1, err)
	}
	return taskqueue.Task{
		Type:    TypeSendTaskV1,
		Payload: data,
		Queue:   taskqueue.QueueDefault,
	}, nil
}

func DecodeDispatchDuePayload(payload []byte) (DispatchDuePayload, error) {
	var row DispatchDuePayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return DispatchDuePayload{}, fmt.Errorf("decode %s payload: %w", TypeDispatchDueV1, err)
	}
	return row, nil
}

func DecodeSendTaskPayload(payload []byte) (SendTaskPayload, error) {
	var row SendTaskPayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return SendTaskPayload{}, fmt.Errorf("decode %s payload: %w", TypeSendTaskV1, err)
	}
	if row.TaskID <= 0 {
		return SendTaskPayload{}, fmt.Errorf("decode %s payload: task_id is required", TypeSendTaskV1)
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

	mux.HandleFunc(TypeDispatchDueV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeDispatchDuePayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.DispatchDue(ctx, DispatchDueInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed notification dispatch due task", "claimed", result.Claimed, "queued", result.Queued)
		return nil
	})

	mux.HandleFunc(TypeSendTaskV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeSendTaskPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.SendTask(ctx, SendTaskInput{TaskID: payload.TaskID})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed notification send task", "task_id", result.TaskID, "sent", result.Sent, "noop", result.Noop)
		return nil
	})
}
