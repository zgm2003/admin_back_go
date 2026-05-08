package aichat

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
)

type Attachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type CreateRunInput struct {
	Content        string
	ConversationID int64
	AgentID        int64
	MaxHistory     int
	Attachments    []Attachment
	Temperature    *float64
	MaxTokens      *int
}

type CreateRunResponse struct {
	ConversationID int64  `json:"conversation_id"`
	RunID          int64  `json:"run_id"`
	RequestID      string `json:"request_id"`
	UserMessageID  int64  `json:"user_message_id"`
	AgentID        int64  `json:"agent_id"`
	IsNew          bool   `json:"is_new"`
}

type StreamEventItem struct {
	ID    string         `json:"id"`
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

type EventsResponse struct {
	Events    []StreamEventItem `json:"events"`
	LastID    string            `json:"last_id"`
	RunStatus int               `json:"run_status"`
	Terminal  bool              `json:"terminal"`
	ErrorMsg  string            `json:"error_msg"`
}

type CancelResponse struct {
	RunID  int64  `json:"run_id"`
	Status string `json:"status"`
}

type RunExecuteInput struct {
	RunID int64
}

type RunExecuteResult struct {
	RunID int64
}

type RunTimeoutInput struct {
	Limit int
}

type RunTimeoutResult struct {
	Failed int64 `json:"failed"`
}

type CreateRunRecord struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	Content        string
	RequestID      string
	MetaJSON       *string
	Now            time.Time
}

type RunStartRecord struct {
	RunID          int64
	ConversationID int64
	RequestID      string
	UserMessageID  int64
	AgentID        int64
	IsNew          bool
}

type RunExecutionRecord struct {
	Run                Run
	UserMessageContent string
}

type RunSuccessRecord struct {
	RunID            int64
	ConversationID   int64
	Content          string
	ModelSnapshot    string
	PromptTokens     int
	CompletionTokens int
	LatencyMS        int
}

type GenerateInput struct {
	RunID   int64
	UserID  int64
	AgentID int64
	Content string
}

type GenerateResult struct {
	Content          string
	ModelSnapshot    string
	PromptTokens     int
	CompletionTokens int
}

type Provider interface {
	Generate(ctx context.Context, input GenerateInput) (*GenerateResult, error)
}

type Repository interface {
	ActiveAgentExists(ctx context.Context, id int64) (bool, error)
	Conversation(ctx context.Context, id int64) (*Conversation, error)
	CreateRun(ctx context.Context, input CreateRunRecord) (*RunStartRecord, error)
	RunForUser(ctx context.Context, runID int64, userID int64) (*Run, error)
	RunForExecute(ctx context.Context, runID int64) (*RunExecutionRecord, error)
	AssistantMessage(ctx context.Context, id int64) (*Message, error)
	MarkCanceled(ctx context.Context, runID int64) error
	MarkSuccess(ctx context.Context, input RunSuccessRecord) (*Message, error)
	MarkFailed(ctx context.Context, runID int64, message string) error
	TimeoutRuns(ctx context.Context, limit int, message string) (int64, error)
}

type HTTPService interface {
	CreateRun(ctx context.Context, userID int64, input CreateRunInput) (*CreateRunResponse, *apperror.Error)
	Events(ctx context.Context, userID int64, runID int64, lastID string) (*EventsResponse, *apperror.Error)
	Cancel(ctx context.Context, userID int64, runID int64) (*CancelResponse, *apperror.Error)
}

type JobService interface {
	ExecuteRun(ctx context.Context, input RunExecuteInput) (*RunExecuteResult, error)
	TimeoutRuns(ctx context.Context, input RunTimeoutInput) (*RunTimeoutResult, error)
}
