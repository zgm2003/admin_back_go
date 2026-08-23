package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
	ContextMemoryBuild       contextengine.MemoryBuildJobService
	ContextConversationIndex contextengine.ConversationIndexJobService
	ContextProfileRebuild    contextengine.ProfileRebuildJobService
	ContextIndexCleanup      contextengine.IndexCleanupJobService
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
		func() error { return registerContextMemoryBuild(registry, deps.ContextMemoryBuild) },
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
