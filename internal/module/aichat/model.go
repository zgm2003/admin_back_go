package aichat

import "time"

type Conversation struct {
	ID                   int64      `gorm:"column:id;primaryKey"`
	AppID                uint64     `gorm:"column:app_id"`
	UserID               int64      `gorm:"column:user_id"`
	AgentID              int64      `gorm:"column:agent_id"`
	Title                string     `gorm:"column:title"`
	EngineConversationID string     `gorm:"column:engine_conversation_id"`
	LastMessageAt        *time.Time `gorm:"column:last_message_at"`
	Status               int        `gorm:"column:status"`
	IsDel                int        `gorm:"column:is_del"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
}

func (Conversation) TableName() string { return "ai_conversations" }

type Message struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	ConversationID  int64     `gorm:"column:conversation_id"`
	RunID           *int64    `gorm:"column:run_id"`
	UserID          int64     `gorm:"column:user_id"`
	Role            int       `gorm:"column:role"`
	ContentType     string    `gorm:"column:content_type"`
	Content         string    `gorm:"column:content"`
	EngineMessageID string    `gorm:"column:engine_message_id"`
	TokenInput      int       `gorm:"column:token_input"`
	TokenOutput     int       `gorm:"column:token_output"`
	MetaJSON        *string   `gorm:"column:meta_json"`
	Status          int       `gorm:"column:status"`
	IsDel           int       `gorm:"column:is_del"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (Message) TableName() string { return "ai_messages" }

type Run struct {
	ID                 int64      `gorm:"column:id;primaryKey"`
	RunUID             string     `gorm:"column:run_uid"`
	AppID              uint64     `gorm:"column:app_id"`
	ConversationID     int64      `gorm:"column:conversation_id"`
	UserID             int64      `gorm:"column:user_id"`
	AgentID            int64      `gorm:"column:agent_id"`
	UserMessageID      *int64     `gorm:"column:user_message_id"`
	AssistantMessageID *int64     `gorm:"column:assistant_message_id"`
	ProviderID         uint64     `gorm:"column:provider_id"`
	EngineTaskID       string     `gorm:"column:engine_task_id"`
	EngineRunID        string     `gorm:"column:engine_run_id"`
	RequestID          string     `gorm:"column:request_id"`
	RunStatus          int        `gorm:"column:run_status"`
	InputSnapshotJSON  *string    `gorm:"column:input_snapshot_json"`
	OutputSnapshotJSON *string    `gorm:"column:output_snapshot_json"`
	UsageJSON          *string    `gorm:"column:usage_json"`
	PromptTokens       int        `gorm:"column:prompt_tokens"`
	CompletionTokens   int        `gorm:"column:completion_tokens"`
	TotalTokens        int        `gorm:"column:total_tokens"`
	Cost               float64    `gorm:"column:cost"`
	LatencyMS          int        `gorm:"column:latency_ms"`
	ErrorCode          string     `gorm:"column:error_code"`
	ErrorMsg           *string    `gorm:"column:error_msg"`
	ModelSnapshot      string     `gorm:"column:model_snapshot"`
	MetaJSON           *string    `gorm:"column:meta_json"`
	StartedAt          *time.Time `gorm:"column:started_at"`
	CompletedAt        *time.Time `gorm:"column:completed_at"`
	CanceledAt         *time.Time `gorm:"column:canceled_at"`
	IsDel              int        `gorm:"column:is_del"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (Run) TableName() string { return "ai_runs" }

type App struct {
	ID                 uint64 `gorm:"column:id;primaryKey"`
	ProviderID         uint64 `gorm:"column:provider_id"`
	Name               string `gorm:"column:name"`
	AppType            string `gorm:"column:app_type"`
	EngineAppID        string `gorm:"column:engine_app_id"`
	EngineAppAPIKeyEnc string `gorm:"column:engine_app_api_key_enc"`
	RuntimeConfigJSON  string `gorm:"column:runtime_config_json"`
	ModelSnapshotJSON  string `gorm:"column:model_snapshot_json"`
	Status             int    `gorm:"column:status"`
	IsDel              int    `gorm:"column:is_del"`
}

func (App) TableName() string { return "ai_apps" }

type Provider struct {
	ID         uint64 `gorm:"column:id;primaryKey"`
	EngineType string `gorm:"column:engine_type"`
	BaseURL    string `gorm:"column:base_url"`
	APIKeyEnc  string `gorm:"column:api_key_enc"`
	Status     int    `gorm:"column:status"`
	IsDel      int    `gorm:"column:is_del"`
}

func (Provider) TableName() string { return "ai_providers" }

type RunEvent struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	RunID       int64     `gorm:"column:run_id"`
	Seq         uint64    `gorm:"column:seq"`
	EventID     string    `gorm:"column:event_id"`
	EventType   string    `gorm:"column:event_type"`
	DeltaText   string    `gorm:"column:delta_text"`
	PayloadJSON string    `gorm:"column:payload_json"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (RunEvent) TableName() string { return "ai_run_events" }
