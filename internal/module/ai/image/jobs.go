package aiimage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const (
	TypeGenerateV1  = "ai:image-generate:v1"
	GenerateTimeout = 10 * time.Minute
)

type GeneratePayload struct {
	Platform string `json:"platform"`
	TaskID   uint64 `json:"task_id"`
	UserID   uint64 `json:"user_id"`
}

type GenerateInput = GeneratePayload

type GenerateResult struct {
	TaskID uint64
	Status string
}

func NewGenerateTask(payload GeneratePayload) (taskqueue.Task, error) {
	payload.Platform = strings.TrimSpace(payload.Platform)
	if !enum.IsRegisteredPlatform(payload.Platform) || payload.TaskID == 0 || payload.UserID == 0 {
		return taskqueue.Task{}, fmt.Errorf("%s payload registered platform, task_id and user_id are required", TypeGenerateV1)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeGenerateV1, err)
	}
	return taskqueue.Task{Type: TypeGenerateV1, Payload: data}, nil
}

func DecodeGeneratePayload(payload []byte) (GeneratePayload, error) {
	var row GeneratePayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return GeneratePayload{}, fmt.Errorf("decode %s payload: %w", TypeGenerateV1, err)
	}
	row.Platform = strings.TrimSpace(row.Platform)
	if !enum.IsRegisteredPlatform(row.Platform) || row.TaskID == 0 || row.UserID == 0 {
		return GeneratePayload{}, fmt.Errorf("decode %s payload: registered platform, task_id and user_id are required", TypeGenerateV1)
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
	return registry.Register(taskqueue.Definition{
		Type:      TypeGenerateV1,
		Queue:     taskqueue.QueueLow,
		Timeout:   GenerateTimeout,
		MaxRetry:  2,
		UniqueTTL: GenerateTimeout,
		Decode: func(data []byte) (any, *apperror.Error) {
			payload, err := DecodeGeneratePayload(data)
			if err != nil {
				return nil, taskqueue.PayloadError(TypeGenerateV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeGenerateV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(GeneratePayload)
			if !ok {
				return taskqueue.InvariantError(TypeGenerateV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			result, err := service.ExecuteGenerate(ctx, payload)
			if err != nil {
				return taskqueue.HandlerError(TypeGenerateV1, err)
			}
			if result == nil {
				return taskqueue.InvariantError(TypeGenerateV1, fmt.Errorf("nil generate result"))
			}
			logger.InfoContext(ctx, "processed ai image generate task", "task_id", result.TaskID, "status", result.Status)
			return nil
		},
	})
}

type WakeupEnqueuer struct{ queue taskqueue.Enqueuer }

func NewWakeupEnqueuer(queue taskqueue.Enqueuer) *WakeupEnqueuer {
	return &WakeupEnqueuer{queue: queue}
}

func (waker *WakeupEnqueuer) WakeImageTask(ctx context.Context, task ImageTask) error {
	if waker == nil || waker.queue == nil {
		return taskqueue.ErrClientNotReady
	}
	queued, err := NewGenerateTask(GeneratePayload{Platform: task.Platform, TaskID: task.ID, UserID: task.UserID})
	if err != nil {
		return err
	}
	_, err = waker.queue.Enqueue(ctx, queued)
	if taskqueue.IsDuplicateTask(err) {
		return nil
	}
	return err
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
