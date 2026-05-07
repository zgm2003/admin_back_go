package exporttask

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	TypeRunV1    = "export:run:v1"
	KindUserList = "user_list"
)

type RunPayload struct {
	TaskID   int64   `json:"task_id"`
	Kind     string  `json:"kind"`
	UserID   int64   `json:"user_id"`
	Platform string  `json:"platform"`
	IDs      []int64 `json:"ids"`
}

type RunInput = RunPayload

type JobService interface {
	Run(ctx context.Context, input RunInput) error
}

func NewRunTask(payload RunPayload) (taskqueue.Task, error) {
	payload = normalizeRunPayload(payload)
	if err := validateRunInput(payload); err != nil {
		return taskqueue.Task{}, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeRunV1, err)
	}
	return taskqueue.Task{Type: TypeRunV1, Payload: data, Queue: taskqueue.QueueLow, MaxRetry: 3, Timeout: 5 * time.Minute}, nil
}

func DecodeRunPayload(data []byte) (RunPayload, error) {
	var payload RunPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return RunPayload{}, fmt.Errorf("decode %s payload: %w", TypeRunV1, err)
	}
	payload = normalizeRunPayload(payload)
	if err := validateRunInput(payload); err != nil {
		return RunPayload{}, err
	}
	return payload, nil
}

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	if mux == nil || service == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	mux.HandleFunc(TypeRunV1, func(ctx context.Context, task taskqueue.Task) error {
		payload, err := DecodeRunPayload(task.Payload)
		if err != nil {
			return err
		}
		if err := service.Run(ctx, payload); err != nil {
			logger.WarnContext(ctx, "export run task failed", "task_id", payload.TaskID, "kind", payload.Kind, "error", err)
			return err
		}
		return nil
	})
}

func normalizeRunPayload(payload RunPayload) RunPayload {
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.Platform = strings.TrimSpace(payload.Platform)
	payload.IDs = normalizeIDs(payload.IDs)
	return payload
}

func validateRunInput(input RunInput) error {
	if input.TaskID <= 0 {
		return fmt.Errorf("%s payload task_id is required", TypeRunV1)
	}
	if strings.TrimSpace(input.Kind) == "" {
		return fmt.Errorf("%s payload kind is required", TypeRunV1)
	}
	if input.UserID <= 0 {
		return fmt.Errorf("%s payload user_id is required", TypeRunV1)
	}
	if len(normalizeIDs(input.IDs)) == 0 {
		return fmt.Errorf("%s payload ids are required", TypeRunV1)
	}
	return nil
}
