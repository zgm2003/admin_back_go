package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
)

const TypeAuthLoginLogV1 = "auth:login-log:v1"

// LoginLogPayload is the queue-safe representation of a login audit row.
type LoginLogPayload struct {
	UserID       *int64 `json:"user_id,omitempty"`
	LoginAccount string `json:"login_account"`
	LoginType    string `json:"login_type"`
	Platform     string `json:"platform"`
	IP           string `json:"ip"`
	UserAgent    string `json:"ua"`
	IsSuccess    int    `json:"is_success"`
	Reason       string `json:"reason,omitempty"`
}

// NewLoginLogTask builds the versioned login-log task.
func NewLoginLogTask(attempt LoginAttempt) (taskqueue.Task, error) {
	data, err := json.Marshal(LoginLogPayload(attempt))
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeAuthLoginLogV1, err)
	}
	return taskqueue.Task{
		Type:    TypeAuthLoginLogV1,
		Payload: data,
	}, nil
}

// DecodeLoginLogPayload decodes a versioned login-log task payload.
func DecodeLoginLogPayload(payload []byte) (LoginAttempt, error) {
	var row LoginLogPayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return LoginAttempt{}, fmt.Errorf("decode %s payload: %w", TypeAuthLoginLogV1, err)
	}
	return LoginAttempt(row), nil
}

// RegisterLoginLogTask registers the complete login-log task contract.
func RegisterLoginLogTask(registry *taskqueue.Registry, repo Repository, logger *slog.Logger) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	if logger == nil {
		logger = slog.Default()
	}

	return registry.Register(taskqueue.Definition{
		Type:     TypeAuthLoginLogV1,
		Queue:    taskqueue.QueueCritical,
		Timeout:  taskqueue.DefaultTimeout,
		MaxRetry: taskqueue.DefaultMaxRetry,
		Decode: func(payload []byte) (any, *apperror.Error) {
			attempt, err := DecodeLoginLogPayload(payload)
			if err != nil {
				return nil, taskqueue.PayloadError(TypeAuthLoginLogV1, err)
			}
			return attempt, nil
		},
		Handle: func(ctx context.Context, payload any) *apperror.Error {
			if repo == nil {
				return taskqueue.InvariantError(TypeAuthLoginLogV1, ErrRepositoryNotConfigured)
			}
			attempt, ok := payload.(LoginAttempt)
			if !ok {
				return taskqueue.InvariantError(TypeAuthLoginLogV1, fmt.Errorf("decoded payload type %T", payload))
			}
			if err := repo.RecordLoginAttempt(ctx, attempt); err != nil {
				return taskqueue.HandlerError(TypeAuthLoginLogV1, fmt.Errorf("record login log: %w", err))
			}
			logger.InfoContext(ctx, "processed login log task", "type", TypeAuthLoginLogV1, "login_type", attempt.LoginType, "is_success", attempt.IsSuccess)
			return nil
		},
	})
}

// RegisterLoginLogHandler remains as a test adapter while all executable
// ownership lives in Registry.
func RegisterLoginLogHandler(mux *taskqueue.Mux, repo Repository, logger *slog.Logger) {
	registry := taskqueue.NewRegistry()
	if err := RegisterLoginLogTask(registry, repo, logger); err != nil {
		panic(err)
	}
	if err := mux.RegisterRegistry(registry); err != nil {
		panic(err)
	}
}
