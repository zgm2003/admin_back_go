package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/platform/scheduler"
	"admin_back_go/internal/platform/taskqueue"
)

const TypeSystemNoopV1 = "system:no-op:v1"

// Dependencies are shared job handler dependencies.
type Dependencies struct {
	Logger         *slog.Logger
	AuthRepository auth.Repository
}

// NoopPayload is the payload for the system no-op probe task.
type NoopPayload struct {
	Message string `json:"message,omitempty"`
}

// Register wires task handlers into the queue mux.
func Register(mux *taskqueue.Mux, deps Dependencies) {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux.HandleFunc(TypeSystemNoopV1, func(ctx context.Context, task taskqueue.Task) error {
		var payload NoopPayload
		if len(task.Payload) > 0 {
			if err := json.Unmarshal(task.Payload, &payload); err != nil {
				return fmt.Errorf("decode %s payload: %w", TypeSystemNoopV1, err)
			}
		}
		logger.InfoContext(ctx, "processed noop task", "type", task.Type, "message", payload.Message)
		return nil
	})
	auth.RegisterLoginLogHandler(mux, deps.AuthRepository, logger)
}

// NewNoopTask builds a versioned queue probe task.
func NewNoopTask(payload NoopPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeSystemNoopV1, err)
	}
	return taskqueue.Task{
		Type:    TypeSystemNoopV1,
		Payload: data,
	}, nil
}

// RegisterSchedules is the single place for cron-to-queue wiring. It is empty
// until a real business schedule exists; fake cron jobs are worse than no cron.
func RegisterSchedules(_ *scheduler.Scheduler, _ taskqueue.Enqueuer, _ *slog.Logger) error {
	return nil
}
