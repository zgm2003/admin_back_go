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
