package aichat

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
	TypeRunTimeoutV1 = "ai:run-timeout:v1"
)

type RunTimeoutPayload struct {
	Limit int `json:"limit,omitempty"`
}

func NewRunTimeoutTask(payload RunTimeoutPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeRunTimeoutV1, err)
	}
	return taskqueue.Task{Type: TypeRunTimeoutV1, Payload: data}, nil
}

func DecodeRunTimeoutPayload(payload []byte) (RunTimeoutPayload, error) {
	var row RunTimeoutPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return RunTimeoutPayload{}, fmt.Errorf("decode %s payload: %w", TypeRunTimeoutV1, err)
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
		Type:      TypeRunTimeoutV1,
		Queue:     taskqueue.QueueDefault,
		Timeout:   taskqueue.DefaultTimeout,
		MaxRetry:  taskqueue.DefaultMaxRetry,
		UniqueTTL: 55 * time.Second,
		Decode: func(data []byte) (any, *apperror.Error) {
			payload, err := DecodeRunTimeoutPayload(data)
			if err != nil {
				return nil, taskqueue.PayloadError(TypeRunTimeoutV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeRunTimeoutV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(RunTimeoutPayload)
			if !ok {
				return taskqueue.InvariantError(TypeRunTimeoutV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			result, err := service.TimeoutRuns(ctx, RunTimeoutInput{Limit: payload.Limit})
			if err != nil {
				return taskqueue.HandlerError(TypeRunTimeoutV1, err)
			}
			if result == nil {
				return taskqueue.InvariantError(TypeRunTimeoutV1, fmt.Errorf("nil timeout result"))
			}
			logger.InfoContext(ctx, "processed ai run timeout task", "failed", result.Failed)
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
