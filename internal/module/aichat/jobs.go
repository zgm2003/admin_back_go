package aichat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	TypeRunExecuteV1 = "ai:run-execute:v1"
	TypeRunTimeoutV1 = "ai:run-timeout:v1"
)

type RunExecutePayload struct {
	RunID int64 `json:"run_id"`
}

type RunTimeoutPayload struct {
	Limit int `json:"limit,omitempty"`
}

func NewRunExecuteTask(payload RunExecutePayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeRunExecuteV1, err)
	}
	return taskqueue.Task{Type: TypeRunExecuteV1, Payload: data, Queue: taskqueue.QueueDefault, MaxRetry: 3, Timeout: 5 * time.Minute}, nil
}

func NewRunTimeoutTask(payload RunTimeoutPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeRunTimeoutV1, err)
	}
	return taskqueue.Task{Type: TypeRunTimeoutV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 55 * time.Second}, nil
}

func DecodeRunExecutePayload(payload []byte) (RunExecutePayload, error) {
	var row RunExecutePayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return RunExecutePayload{}, fmt.Errorf("decode %s payload: %w", TypeRunExecuteV1, err)
	}
	if row.RunID <= 0 {
		return RunExecutePayload{}, fmt.Errorf("decode %s payload: run_id is required", TypeRunExecuteV1)
	}
	return row, nil
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

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	if mux == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	mux.HandleFunc(TypeRunExecuteV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeRunExecutePayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.ExecuteRun(ctx, RunExecuteInput{RunID: payload.RunID})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed ai run execute task", "run_id", result.RunID)
		return nil
	})
	mux.HandleFunc(TypeRunTimeoutV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeRunTimeoutPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.TimeoutRuns(ctx, RunTimeoutInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed ai run timeout task", "failed", result.Failed)
		return nil
	})
}
