package aitext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
)

const (
	TypeGenerateV1  = "ai:text-generate:v1"
	GenerateTimeout = 15 * time.Minute
	// Leave enough exponential-backoff attempts to cross the dispatch deadline
	// before a crashed worker's dispatched attempt is fenced as unknown.
	GenerateMaxRetry = 6
)

type GeneratePayload struct {
	TaskID uint64 `json:"task_id"`
}

type JobService interface {
	ExecuteTask(context.Context, uint64) error
}

func NewGenerateTask(payload GeneratePayload) (taskqueue.Task, error) {
	if payload.TaskID == 0 {
		return taskqueue.Task{}, errors.New("text task id is required")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeGenerateV1, err)
	}
	return taskqueue.Task{Type: TypeGenerateV1, Payload: raw}, nil
}

func RegisterTaskDefinitions(registry *taskqueue.Registry, service JobService, logger *slog.Logger) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	if logger == nil {
		logger = slog.Default()
	}
	return registry.Register(taskqueue.Definition{
		Type: TypeGenerateV1, Queue: taskqueue.QueueDefault, Timeout: GenerateTimeout, MaxRetry: GenerateMaxRetry, UniqueTTL: GenerateTimeout,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload GeneratePayload
			if err := json.Unmarshal(data, &payload); err != nil || payload.TaskID == 0 {
				if err == nil {
					err = errors.New("task_id is required")
				}
				return nil, taskqueue.PayloadError(TypeGenerateV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeGenerateV1, ErrStoreNotConfigured)
			}
			payload, ok := decoded.(GeneratePayload)
			if !ok {
				return taskqueue.InvariantError(TypeGenerateV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			if err := service.ExecuteTask(ctx, payload.TaskID); err != nil {
				return taskqueue.HandlerError(TypeGenerateV1, err)
			}
			logger.InfoContext(ctx, "processed AI text task", "task_id", payload.TaskID)
			return nil
		},
	})
}

type WakeupEnqueuer struct{ queue taskqueue.Enqueuer }

func NewWakeupEnqueuer(queue taskqueue.Enqueuer) *WakeupEnqueuer {
	return &WakeupEnqueuer{queue: queue}
}

func (w *WakeupEnqueuer) WakeTextTask(ctx context.Context, taskID uint64) error {
	if w == nil || w.queue == nil {
		return taskqueue.ErrClientNotReady
	}
	task, err := NewGenerateTask(GeneratePayload{TaskID: taskID})
	if err != nil {
		return err
	}
	_, err = w.queue.Enqueue(ctx, task)
	if taskqueue.IsDuplicateTask(err) {
		return nil
	}
	return err
}
