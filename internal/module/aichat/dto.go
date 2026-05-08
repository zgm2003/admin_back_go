package aichat

import (
	"context"
	"encoding/json"
	"time"

	"admin_back_go/internal/apperror"
	platformai "admin_back_go/internal/platform/ai"
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
	AppID          uint64
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
	AppID          uint64 `json:"app_id"`
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
	AppID          uint64
	IsNew          bool
}

type RunExecutionRecord struct {
	Run                Run
	UserMessageContent string
	App                AppEngineConfig
}

type RunSuccessRecord struct {
	RunID                int64
	ConversationID       int64
	Content              string
	EngineConversationID string
	EngineMessageID      string
	EngineTaskID         string
	EngineRunID          string
	UsageJSON            string
	OutputSnapshotJSON   string
	ModelSnapshot        string
	PromptTokens         int
	CompletionTokens     int
	TotalTokens          int
	Cost                 float64
	LatencyMS            int
}

type EngineConfig struct {
	EngineType platformai.EngineType
	BaseURL    string
	APIKey     string
}

type EngineFactory interface {
	NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error)
}

type AppEngineConfig struct {
	AppID                uint64
	AppName              string
	AppType              string
	EngineConnectionID   uint64
	EngineType           string
	EngineBaseURL        string
	EngineAPIKeyEnc      string
	EngineAppID          string
	EngineAppAPIKeyEnc   string
	RuntimeConfigJSON    string
	ModelSnapshotJSON    string
	ConversationEngineID string
	AppStatus            int
	EngineStatus         int
}

type RunEventRecord struct {
	RunID       int64
	Seq         uint64
	EventID     string
	EventType   string
	DeltaText   string
	PayloadJSON json.RawMessage
	CreatedAt   time.Time
}

type Repository interface {
	ActiveAgentExists(ctx context.Context, id int64) (bool, error)
	DefaultActiveApp(ctx context.Context) (*AppEngineConfig, error)
	AppForRuntime(ctx context.Context, appID uint64) (*AppEngineConfig, error)
	Conversation(ctx context.Context, id int64) (*Conversation, error)
	CreateRun(ctx context.Context, input CreateRunRecord) (*RunStartRecord, error)
	RunForUser(ctx context.Context, runID int64, userID int64) (*Run, error)
	RunForExecute(ctx context.Context, runID int64) (*RunExecutionRecord, error)
	AssistantMessage(ctx context.Context, id int64) (*Message, error)
	MarkCanceled(ctx context.Context, runID int64, message string) error
	MarkSuccess(ctx context.Context, input RunSuccessRecord) (*Message, error)
	MarkFailed(ctx context.Context, runID int64, message string) error
	AppendRunEvent(ctx context.Context, input RunEventRecord) error
	ListRunEvents(ctx context.Context, runID int64) ([]RunEventRecord, error)
	TimeoutRuns(ctx context.Context, limit int, message string) (int64, error)
}

type HTTPService interface {
	CreateRun(ctx context.Context, userID int64, input CreateRunInput) (*CreateRunResponse, *apperror.Error)
	Events(ctx context.Context, userID int64, runID int64, lastID string, timeout time.Duration) (*EventsResponse, *apperror.Error)
	Cancel(ctx context.Context, userID int64, runID int64) (*CancelResponse, *apperror.Error)
}

type JobService interface {
	ExecuteRun(ctx context.Context, input RunExecuteInput) (*RunExecuteResult, error)
	TimeoutRuns(ctx context.Context, input RunTimeoutInput) (*RunTimeoutResult, error)
}
