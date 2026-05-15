package aiimage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const TypeGenerateV1 = "ai:image-generate:v1"

type GeneratePayload struct {
	TaskID uint64 `json:"task_id"`
	UserID uint64 `json:"user_id"`
}

type GenerateInput = GeneratePayload

type GenerateResult struct {
	TaskID uint64
	Status string
}

func NewGenerateTask(payload GeneratePayload) (taskqueue.Task, error) {
	if payload.TaskID == 0 || payload.UserID == 0 {
		return taskqueue.Task{}, fmt.Errorf("%s payload task_id and user_id are required", TypeGenerateV1)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeGenerateV1, err)
	}
	return taskqueue.Task{Type: TypeGenerateV1, Payload: data, Queue: taskqueue.QueueLow, MaxRetry: 2, Timeout: 10 * time.Minute}, nil
}

func DecodeGeneratePayload(payload []byte) (GeneratePayload, error) {
	var row GeneratePayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return GeneratePayload{}, fmt.Errorf("decode %s payload: %w", TypeGenerateV1, err)
	}
	if row.TaskID == 0 || row.UserID == 0 {
		return GeneratePayload{}, fmt.Errorf("decode %s payload: task_id and user_id are required", TypeGenerateV1)
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
	mux.HandleFunc(TypeGenerateV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeGeneratePayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.ExecuteGenerate(ctx, payload)
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed ai image generate task", "task_id", result.TaskID, "status", result.Status)
		return nil
	})
}
