package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
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
		Type:    TypeDispatchDueV1,
		Payload: data,
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

func RegisterTaskDefinitions(registry *taskqueue.Registry, service JobService, logger *slog.Logger) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	if logger == nil {
		logger = slog.Default()
	}

	if err := registry.Register(taskqueue.Definition{
		Type:      TypeDispatchDueV1,
		Queue:     taskqueue.QueueDefault,
		Timeout:   taskqueue.DefaultTimeout,
		MaxRetry:  taskqueue.DefaultMaxRetry,
		UniqueTTL: 55 * time.Second,
		Decode: func(data []byte) (any, *apperror.Error) {
			payload, err := DecodeDispatchDuePayload(data)
			if err != nil {
				return nil, taskqueue.PayloadError(TypeDispatchDueV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeDispatchDueV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(DispatchDuePayload)
			if !ok {
				return taskqueue.InvariantError(TypeDispatchDueV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			result, err := service.DispatchDue(ctx, DispatchDueInput{Limit: payload.Limit})
			if err != nil {
				return taskqueue.HandlerError(TypeDispatchDueV1, err)
			}
			if result == nil {
				return taskqueue.InvariantError(TypeDispatchDueV1, fmt.Errorf("nil dispatch result"))
			}
			logger.InfoContext(ctx, "processed notification dispatch due task", "claimed", result.Claimed, "queued", result.Queued)
			return nil
		},
	}); err != nil {
		return err
	}

	return registry.Register(taskqueue.Definition{
		Type:     TypeSendTaskV1,
		Queue:    taskqueue.QueueDefault,
		Timeout:  taskqueue.DefaultTimeout,
		MaxRetry: taskqueue.DefaultMaxRetry,
		Decode: func(data []byte) (any, *apperror.Error) {
			payload, err := DecodeSendTaskPayload(data)
			if err != nil {
				return nil, taskqueue.PayloadError(TypeSendTaskV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeSendTaskV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(SendTaskPayload)
			if !ok {
				return taskqueue.InvariantError(TypeSendTaskV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			result, err := service.SendTask(ctx, SendTaskInput(payload))
			if err != nil {
				return taskqueue.HandlerError(TypeSendTaskV1, err)
			}
			if result == nil {
				return taskqueue.InvariantError(TypeSendTaskV1, fmt.Errorf("nil send result"))
			}
			logger.InfoContext(ctx, "processed notification send task", "task_id", result.TaskID, "sent", result.Sent, "noop", result.Noop)
			return nil
		},
	})
}

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	registry := taskqueue.NewRegistry()
	if err := RegisterTaskDefinitions(registry, service, logger); err != nil {
		panic(err)
	}
	if err := mux.RegisterRegistry(registry); err != nil {
		panic(err)
	}
}
