package exporttask

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const (
	TypeRunV1            = "export:run:v1"
	TypeCleanupExpiredV1 = "export:cleanup-expired:v1"
	KindUserList         = "user_list"
)

type RunPayload struct {
	TaskID   int64           `json:"task_id"`
	Kind     string          `json:"kind"`
	UserID   int64           `json:"user_id"`
	Platform string          `json:"platform"`
	Scope    string          `json:"scope"`
	IDs      []int64         `json:"ids,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
}

type RunInput = RunPayload

type JobService interface {
	Run(ctx context.Context, input RunInput) error
	CleanupExpired(ctx context.Context) error
}

func NewCleanupExpiredTask() (taskqueue.Task, error) {
	data, err := json.Marshal(struct{}{})
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeCleanupExpiredV1, err)
	}
	return taskqueue.Task{Type: TypeCleanupExpiredV1, Payload: data}, nil
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
	return taskqueue.Task{Type: TypeRunV1, Payload: data}, nil
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

func RegisterTaskDefinitions(registry *taskqueue.Registry, service JobService, logger *slog.Logger) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := registry.Register(taskqueue.Definition{
		Type:     TypeRunV1,
		Queue:    taskqueue.QueueLow,
		Timeout:  5 * time.Minute,
		MaxRetry: 3,
		Decode: func(data []byte) (any, *apperror.Error) {
			payload, err := DecodeRunPayload(data)
			if err != nil {
				return nil, taskqueue.PayloadError(TypeRunV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, decoded any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeRunV1, ErrRepositoryNotConfigured)
			}
			payload, ok := decoded.(RunPayload)
			if !ok {
				return taskqueue.InvariantError(TypeRunV1, fmt.Errorf("decoded payload type %T", decoded))
			}
			if err := service.Run(ctx, payload); err != nil {
				logger.WarnContext(ctx, "export run task failed", "task_id", payload.TaskID, "kind", payload.Kind)
				return taskqueue.HandlerError(TypeRunV1, err)
			}
			return nil
		},
	}); err != nil {
		return err
	}
	return registry.Register(taskqueue.Definition{
		Type:      TypeCleanupExpiredV1,
		Queue:     taskqueue.QueueLow,
		Timeout:   time.Minute,
		MaxRetry:  3,
		UniqueTTL: 5 * time.Minute,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload struct{}
			if err := json.Unmarshal(data, &payload); err != nil {
				return nil, taskqueue.PayloadError(TypeCleanupExpiredV1, err)
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, _ any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeCleanupExpiredV1, ErrRepositoryNotConfigured)
			}
			if err := service.CleanupExpired(ctx); err != nil {
				logger.WarnContext(ctx, "export cleanup expired task failed")
				return taskqueue.HandlerError(TypeCleanupExpiredV1, err)
			}
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

func normalizeRunPayload(payload RunPayload) RunPayload {
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.Platform = normalizePlatform(payload.Platform)
	payload.Scope = strings.TrimSpace(payload.Scope)
	if payload.Scope == "" && payload.Kind == KindUserList {
		payload.Scope = ScopeSelected
	}
	payload.IDs = normalizeIDs(payload.IDs)
	return payload
}

func validateRunInput(input RunInput) error {
	if input.TaskID <= 0 {
		return fmt.Errorf("%s payload task_id is required", TypeRunV1)
	}
	if input.Kind == "" {
		return fmt.Errorf("%s payload kind is required", TypeRunV1)
	}
	if input.UserID <= 0 {
		return fmt.Errorf("%s payload user_id is required", TypeRunV1)
	}
	if !enum.IsRegisteredPlatform(input.Platform) {
		return fmt.Errorf("%s payload platform is not registered: %q", TypeRunV1, input.Platform)
	}
	switch input.Scope {
	case ScopeSelected:
		if len(normalizeIDs(input.IDs)) == 0 {
			return fmt.Errorf("%s payload ids are required", TypeRunV1)
		}
	case ScopeFiltered:
	default:
		return fmt.Errorf("%s payload scope is required", TypeRunV1)
	}
	return nil
}
