package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/infra/scheduler"
	"admin_back_go/internal/infra/taskqueue"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/contextengine"
	aiimage "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/module/ai/replycommand"
	aitext "admin_back_go/internal/module/ai/text"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/export"
	notificationtask "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/module/payment"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/apperror"
)

const TypeSystemNoopV1 = "system:no-op:v1"

var (
	ErrScheduleRegistrarRequired   = errors.New("schedule registrar is required")
	ErrScheduleEnqueuerRequired    = errors.New("schedule enqueuer is required")
	ErrScheduleTaskBuilderRequired = errors.New("schedule task builder is required")
)

// Dependencies are shared job handler dependencies.
type Dependencies struct {
	Logger                   *slog.Logger
	AuthRepository           auth.Repository
	AIChatService            aichat.JobService
	AITextService            aitext.JobService
	AIReplyRunner            replycommand.JobRunner
	AiImageService           aiimage.JobService
	ExportTaskService        exporttask.JobService
	NotificationTaskService  notificationtask.JobService
	PaymentService           payment.JobService
	RealtimeRetentionService modulerealtime.JobService
	ContextDocumentIndex     contextengine.DocumentIndexJobService
	ContextConversationIndex contextengine.ConversationIndexJobService
	ContextProfileRebuild    contextengine.ProfileRebuildJobService
	ContextIndexCleanup      contextengine.IndexCleanupJobService
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

// NewRegistry builds the one complete executable task registry used by both
// producers (policy lookup) and the Worker (decode/handle execution).
func NewRegistry(deps Dependencies) (*taskqueue.Registry, error) {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	registry := taskqueue.NewRegistry()
	if err := registry.Register(taskqueue.Definition{
		Type:     TypeSystemNoopV1,
		Queue:    taskqueue.QueueDefault,
		Timeout:  taskqueue.DefaultTimeout,
		MaxRetry: taskqueue.DefaultMaxRetry,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload NoopPayload
			if len(data) > 0 {
				if err := json.Unmarshal(data, &payload); err != nil {
					return nil, taskqueue.PayloadError(TypeSystemNoopV1, err)
				}
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, _ any) *apperror.Error {
			logger.InfoContext(ctx, "processed noop task", "type", TypeSystemNoopV1)
			return nil
		},
	}); err != nil {
		return nil, err
	}
	for _, register := range []func() error{
		func() error { return auth.RegisterLoginLogTask(registry, deps.AuthRepository, logger) },
		func() error { return replycommand.RegisterTaskDefinition(registry, deps.AIReplyRunner) },
		func() error { return aichat.RegisterTaskDefinitions(registry, deps.AIChatService, logger) },
		func() error { return aitext.RegisterTaskDefinitions(registry, deps.AITextService, logger) },
		func() error { return aiimage.RegisterTaskDefinitions(registry, deps.AiImageService, logger) },
		func() error { return exporttask.RegisterTaskDefinitions(registry, deps.ExportTaskService, logger) },
		func() error {
			return notificationtask.RegisterTaskDefinitions(registry, deps.NotificationTaskService, logger)
		},
		func() error { return payment.RegisterTaskDefinitions(registry, deps.PaymentService, logger) },
		func() error {
			return modulerealtime.RegisterTaskDefinitions(registry, deps.RealtimeRetentionService, logger)
		},
		func() error { return registerContextDocumentIndex(registry, deps.ContextDocumentIndex) },
		func() error { return registerContextConversationIndex(registry, deps.ContextConversationIndex) },
		func() error { return registerContextProfileRebuild(registry, deps.ContextProfileRebuild) },
		func() error { return registerContextIndexCleanup(registry, deps.ContextIndexCleanup) },
	} {
		if err := register(); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register is a test-compatible mux adapter; task ownership remains in the
// registry returned by NewRegistry.
func Register(mux *taskqueue.Mux, deps Dependencies) {
	registry, err := NewRegistry(deps)
	if err != nil {
		panic(err)
	}
	if err := mux.RegisterRegistry(registry); err != nil {
		panic(err)
	}
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
