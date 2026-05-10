package aichat

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	platformai "admin_back_go/internal/platform/ai"
)

type ConversationReplyInput = ConversationReplyPayload

type ConversationReplyResult struct {
	ConversationID     int64
	AssistantMessageID int64
}

type RunTimeoutInput struct {
	Limit int
}

type RunTimeoutResult struct {
	Failed int64 `json:"failed"`
}

type AssistantMessageRecord struct {
	ConversationID int64
	Content        string
	Now            time.Time
}

type EngineConfig struct {
	EngineType platformai.EngineType
	BaseURL    string
	APIKey     string
}

type EngineFactory interface {
	NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error)
}

type AgentEngineConfig struct {
	AgentID          uint64
	AgentName        string
	ModelID          string
	ModelDisplayName string
	SystemPrompt     string
	ScenesJSON       string
	ProviderID       uint64
	EngineType       string
	EngineBaseURL    string
	EngineAPIKeyEnc  string
	AgentStatus      int
	EngineStatus     int
}

type MessageHistory struct {
	ID          int64
	Role        int
	ContentType string
	Content     string
	MetaJSON    *string
	CreatedAt   time.Time
}

type Repository interface {
	ConversationForReply(ctx context.Context, id int64, userID int64) (*Conversation, error)
	AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error)
	LatestMessages(ctx context.Context, conversationID int64, limit int) ([]MessageHistory, error)
	InsertAssistantMessage(ctx context.Context, input AssistantMessageRecord) (int64, error)
	TimeoutRuns(ctx context.Context, limit int, message string) (int64, error)
}

type HTTPService interface{}

type JobService interface {
	ExecuteConversationReply(ctx context.Context, input ConversationReplyInput) (*ConversationReplyResult, error)
	TimeoutRuns(ctx context.Context, input RunTimeoutInput) (*RunTimeoutResult, error)
}

type appError = apperror.Error
