package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/aiimage"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/exporttask"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/platform/scheduler"
	"admin_back_go/internal/platform/taskqueue"
)

const TypeSystemNoopV1 = "system:no-op:v1"

var (
	ErrScheduleRegistrarRequired   = errors.New("schedule registrar is required")
	ErrScheduleEnqueuerRequired    = errors.New("schedule enqueuer is required")
	ErrScheduleTaskBuilderRequired = errors.New("schedule task builder is required")
)

// Dependencies are shared job handler dependencies.
type Dependencies struct {
	Logger                  *slog.Logger
	AuthRepository          auth.Repository
	AIChatService           aichat.JobService
	AIImageService          aiimage.JobService
	ExportTaskService       exporttask.JobService
	NotificationTaskService notificationtask.JobService
}

// ScheduleRegistrar is the worker-owned boundary used by job schedule
// registration. It exists so tests can prove schedules enqueue tasks without
// depending on gocron internals.
type ScheduleRegistrar interface {
	Every(name string, interval time.Duration, task scheduler.TaskFunc) error
	Cron(name string, expression string, withSeconds bool, task scheduler.TaskFunc) error
}

// ScheduledTaskDefinition describes a cron/interval trigger that only builds a
// queue task. Business work must stay in the worker handler for that task type.
type ScheduledTaskDefinition struct {
	Name        string
	Every       time.Duration
	Cron        string
	WithSeconds bool
	BuildTask   func() (taskqueue.Task, error)
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
	aichat.RegisterHandlers(mux, deps.AIChatService, logger)
	aiimage.RegisterHandlers(mux, deps.AIImageService, logger)
	exporttask.RegisterHandlers(mux, deps.ExportTaskService, logger)
	notificationtask.RegisterHandlers(mux, deps.NotificationTaskService, logger)
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

// RegisterSchedules intentionally registers no static schedule. DB-backed cron
// tasks are owned by internal/module/crontask.SchedulerService so the System
// Management page remains the runtime truth for enabled schedules.
func RegisterSchedules(registrar ScheduleRegistrar, enqueuer taskqueue.Enqueuer, logger *slog.Logger) error {
	return registerScheduleDefinitions(registrar, enqueuer, logger, nil)
}

func registerScheduleDefinitions(registrar ScheduleRegistrar, enqueuer taskqueue.Enqueuer, logger *slog.Logger, definitions []ScheduledTaskDefinition) error {
	if registrar == nil {
		return ErrScheduleRegistrarRequired
	}
	if enqueuer == nil {
		return ErrScheduleEnqueuerRequired
	}
	if logger == nil {
		logger = slog.Default()
	}

	for _, definition := range definitions {
		task, err := scheduledEnqueueTask(definition, enqueuer, logger)
		if err != nil {
			return err
		}

		if definition.Every > 0 {
			if err := registrar.Every(definition.Name, definition.Every, task); err != nil {
				return fmt.Errorf("register interval schedule %s: %w", definition.Name, err)
			}
			continue
		}
		if strings.TrimSpace(definition.Cron) != "" {
			if err := registrar.Cron(definition.Name, definition.Cron, definition.WithSeconds, task); err != nil {
				return fmt.Errorf("register cron schedule %s: %w", definition.Name, err)
			}
			continue
		}
		return fmt.Errorf("register schedule %s: %w", definition.Name, scheduler.ErrJobIntervalRequired)
	}
	return nil
}

func scheduledEnqueueTask(definition ScheduledTaskDefinition, enqueuer taskqueue.Enqueuer, logger *slog.Logger) (scheduler.TaskFunc, error) {
	if definition.BuildTask == nil {
		return nil, fmt.Errorf("%w: %s", ErrScheduleTaskBuilderRequired, definition.Name)
	}

	return func(ctx context.Context) error {
		task, err := definition.BuildTask()
		if err != nil {
			return fmt.Errorf("build schedule %s task: %w", definition.Name, err)
		}
		result, err := enqueuer.Enqueue(ctx, task)
		if err != nil {
			return fmt.Errorf("enqueue schedule %s task %s: %w", definition.Name, task.Type, err)
		}
		logger.InfoContext(ctx, "scheduled task enqueued", "schedule", definition.Name, "task_type", result.Type, "task_id", result.ID, "queue", result.Queue)
		return nil
	}, nil
}
