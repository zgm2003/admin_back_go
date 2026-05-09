package aichat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/platform/taskqueue"
)

const (
	ConversationReplyTaskName = "ai:conversation-reply:v1"
	TypeRunTimeoutV1          = "ai:run-timeout:v1"
)

type ConversationReplyPayload struct {
	ConversationID int64  `json:"conversation_id"`
	UserID         int64  `json:"user_id"`
	AgentID        int64  `json:"agent_id"`
	UserMessageID  int64  `json:"user_message_id"`
	RequestID      string `json:"request_id"`
}

type RunTimeoutPayload struct {
	Limit int `json:"limit,omitempty"`
}

func NewConversationReplyTask(payload ConversationReplyPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", ConversationReplyTaskName, err)
	}
	return taskqueue.Task{Type: ConversationReplyTaskName, Payload: data, Queue: taskqueue.QueueDefault, MaxRetry: 2, Timeout: 5 * time.Minute}, nil
}

func NewRunTimeoutTask(payload RunTimeoutPayload) (taskqueue.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeRunTimeoutV1, err)
	}
	return taskqueue.Task{Type: TypeRunTimeoutV1, Payload: data, Queue: taskqueue.QueueDefault, UniqueTTL: 55 * time.Second}, nil
}

func DecodeConversationReplyPayload(payload []byte) (ConversationReplyPayload, error) {
	var row ConversationReplyPayload
	if err := json.Unmarshal(payload, &row); err != nil {
		return ConversationReplyPayload{}, fmt.Errorf("decode %s payload: %w", ConversationReplyTaskName, err)
	}
	if row.ConversationID <= 0 || row.UserID <= 0 || row.AgentID <= 0 || row.UserMessageID <= 0 || row.RequestID == "" {
		return ConversationReplyPayload{}, fmt.Errorf("decode %s payload: conversation_id, user_id, agent_id, user_message_id and request_id are required", ConversationReplyTaskName)
	}
	return row, nil
}

func DecodeRunTimeoutPayload(payload []byte) (RunTimeoutPayload, error) {
	var row RunTimeoutPayload
	if len(payload) == 0 {
		return row, nil
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		return RunTimeoutPayload{}, fmt.Errorf("decode %s payload: %w", TypeRunTimeoutV1, err)
	}
	return row, nil
}

func RegisterHandlers(mux *taskqueue.Mux, service JobService, logger *slog.Logger) {
	if mux == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	mux.HandleFunc(ConversationReplyTaskName, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeConversationReplyPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.ExecuteConversationReply(ctx, payload)
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed ai conversation reply task", "conversation_id", result.ConversationID, "assistant_message_id", result.AssistantMessageID)
		return nil
	})
	mux.HandleFunc(TypeRunTimeoutV1, func(ctx context.Context, task taskqueue.Task) error {
		if service == nil {
			return ErrRepositoryNotConfigured
		}
		payload, err := DecodeRunTimeoutPayload(task.Payload)
		if err != nil {
			return err
		}
		result, err := service.TimeoutRuns(ctx, RunTimeoutInput{Limit: payload.Limit})
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "processed ai run timeout task", "failed", result.Failed)
		return nil
	})
}
