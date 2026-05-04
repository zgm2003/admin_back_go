package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"admin_back_go/internal/platform/taskqueue"
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
	data, err := json.Marshal(LoginLogPayload{
		UserID:       attempt.UserID,
		LoginAccount: attempt.LoginAccount,
		LoginType:    attempt.LoginType,
		Platform:     attempt.Platform,
		IP:           attempt.IP,
		UserAgent:    attempt.UserAgent,
		IsSuccess:    attempt.IsSuccess,
		Reason:       attempt.Reason,
	})
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeAuthLoginLogV1, err)
	}
	return taskqueue.Task{
		Type:    TypeAuthLoginLogV1,
		Payload: data,
		Queue:   taskqueue.QueueCritical,
	}, nil
}

// DecodeLoginLogPayload decodes a versioned login-log task payload.
func DecodeLoginLogPayload(payload []byte) (LoginAttempt, error) {
	var row LoginLogPayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return LoginAttempt{}, fmt.Errorf("decode %s payload: %w", TypeAuthLoginLogV1, err)
	}
	return LoginAttempt{
		UserID:       row.UserID,
		LoginAccount: row.LoginAccount,
		LoginType:    row.LoginType,
		Platform:     row.Platform,
		IP:           row.IP,
		UserAgent:    row.UserAgent,
		IsSuccess:    row.IsSuccess,
		Reason:       row.Reason,
	}, nil
}

// RegisterLoginLogHandler wires auth login-log consumption into the queue mux.
func RegisterLoginLogHandler(mux *taskqueue.Mux, repo Repository, logger *slog.Logger) {
	if mux == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	mux.HandleFunc(TypeAuthLoginLogV1, func(ctx context.Context, task taskqueue.Task) error {
		if repo == nil {
			return ErrRepositoryNotConfigured
		}
		attempt, err := DecodeLoginLogPayload(task.Payload)
		if err != nil {
			return err
		}
		if err := repo.RecordLoginAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("record login log: %w", err)
		}
		logger.InfoContext(ctx, "processed login log task", "type", task.Type, "login_type", attempt.LoginType, "is_success", attempt.IsSuccess)
		return nil
	})
}
