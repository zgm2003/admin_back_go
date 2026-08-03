package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/apperror"
)

const (
	TaskContextDocumentIndexV1   = contextengine.TaskContextDocumentIndexV1
	TaskContextProfileRebuildV1  = contextengine.TaskContextProfileRebuildV1
	TaskContextIndexCleanupV1    = contextengine.TaskContextIndexCleanupV1
	ContextDocumentIndexMaxRetry = contextengine.DocumentIndexMaxRetry
	ContextDocumentIndexTimeout  = 3 * time.Minute
)

type ContextDocumentIndexV1 = contextengine.ContextDocumentIndexV1

type contextDocumentIndexInvocation struct {
	payload contextengine.ContextDocumentIndexV1
	attempt contextengine.DocumentIndexAttempt
}

func registerContextDocumentIndex(registry *taskqueue.Registry, service contextengine.DocumentIndexJobService) error {
	return registry.Register(taskqueue.Definition{
		Type: TaskContextDocumentIndexV1, Queue: taskqueue.QueueLow,
		Timeout: ContextDocumentIndexTimeout, MaxRetry: ContextDocumentIndexMaxRetry,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload ContextDocumentIndexV1
			if err := json.Unmarshal(data, &payload); err != nil || payload.DocumentVersionID == 0 {
				return nil, taskqueue.PayloadError(TaskContextDocumentIndexV1, errors.New("document_version_id must be positive"))
			}
			return &contextDocumentIndexInvocation{payload: payload}, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TaskContextDocumentIndexV1, errors.New("context document index service is required"))
			}
			invocation, ok := decoded.(*contextDocumentIndexInvocation)
			if !ok {
				return taskqueue.InvariantError(TaskContextDocumentIndexV1, errors.New("unexpected payload type"))
			}
			var err error
			invocation.attempt, err = service.IndexDocument(ctx, invocation.payload.DocumentVersionID)
			return taskqueue.HandlerError(TaskContextDocumentIndexV1, err)
		},
		FinalizeExhausted: func(ctx context.Context, decoded any, _ *apperror.Error, delivery taskqueue.Attempt) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TaskContextDocumentIndexV1+".finalize", errors.New("context document index service is required"))
			}
			invocation, ok := decoded.(*contextDocumentIndexInvocation)
			if !ok {
				return taskqueue.InvariantError(TaskContextDocumentIndexV1+".finalize", errors.New("unexpected payload type"))
			}
			if err := service.FinalizeDocumentIndex(ctx, invocation.attempt, "ai.context.index_retry_exhausted", delivery.Limit); err != nil {
				return apperror.Wrap("ai.context.index_finalize_failed", apperror.CategoryInternal, http.StatusInternalServerError,
					apperror.Retryable, "", nil, "context index finalization failed", err)
			}
			return nil
		},
	})
}

func registerContextProfileRebuild(registry *taskqueue.Registry, service contextengine.ProfileRebuildJobService) error {
	return registry.Register(taskqueue.Definition{
		Type: TaskContextProfileRebuildV1, Queue: taskqueue.QueueLow, Timeout: 30 * time.Minute, MaxRetry: 3,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload contextengine.ContextProfileRebuildV1
			if err := json.Unmarshal(data, &payload); err != nil || payload.ProfileID == 0 {
				return nil, taskqueue.PayloadError(TaskContextProfileRebuildV1, errors.New("profile_id must be positive"))
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TaskContextProfileRebuildV1, errors.New("context profile rebuild service is required"))
			}
			payload, ok := decoded.(contextengine.ContextProfileRebuildV1)
			if !ok {
				return taskqueue.InvariantError(TaskContextProfileRebuildV1, errors.New("unexpected payload type"))
			}
			return taskqueue.HandlerError(TaskContextProfileRebuildV1, service.RebuildProfile(ctx, payload.ProfileID))
		},
	})
}

func registerContextIndexCleanup(registry *taskqueue.Registry, service contextengine.IndexCleanupJobService) error {
	return registry.Register(taskqueue.Definition{
		Type: TaskContextIndexCleanupV1, Queue: taskqueue.QueueLow, Timeout: 5 * time.Minute, MaxRetry: 3,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload contextengine.ContextIndexCleanupV1
			if err := json.Unmarshal(data, &payload); err != nil || payload.Validate() != nil {
				return nil, taskqueue.PayloadError(TaskContextIndexCleanupV1, errors.New("cleanup payload is invalid"))
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TaskContextIndexCleanupV1, errors.New("context index cleanup service is required"))
			}
			payload, ok := decoded.(contextengine.ContextIndexCleanupV1)
			if !ok {
				return taskqueue.InvariantError(TaskContextIndexCleanupV1, errors.New("unexpected payload type"))
			}
			return taskqueue.HandlerError(TaskContextIndexCleanupV1, service.CleanupIndex(ctx, payload))
		},
	})
}
