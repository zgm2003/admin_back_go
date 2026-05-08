package airun

import "time"

type Run struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	RunUID             string    `gorm:"column:run_uid"`
	RequestID          string    `gorm:"column:request_id"`
	AppID              int64     `gorm:"column:app_id"`
	EngineConnectionID int64     `gorm:"column:engine_connection_id"`
	EngineTaskID       string    `gorm:"column:engine_task_id"`
	EngineRunID        string    `gorm:"column:engine_run_id"`
	UserID             int64     `gorm:"column:user_id"`
	ConversationID     int64     `gorm:"column:conversation_id"`
	UserMessageID      *int64    `gorm:"column:user_message_id"`
	AssistantMessageID *int64    `gorm:"column:assistant_message_id"`
	RunStatus          int       `gorm:"column:run_status"`
	ErrorCode          string    `gorm:"column:error_code"`
	ErrorMsg           *string   `gorm:"column:error_msg"`
	PromptTokens       *int      `gorm:"column:prompt_tokens"`
	CompletionTokens   *int      `gorm:"column:completion_tokens"`
	TotalTokens        *int      `gorm:"column:total_tokens"`
	Cost               *float64  `gorm:"column:cost"`
	LatencyMS          *int      `gorm:"column:latency_ms"`
	ModelSnapshot      *string   `gorm:"column:model_snapshot"`
	MetaJSON           *string   `gorm:"column:meta_json"`
	UsageJSON          *string   `gorm:"column:usage_json"`
	OutputSnapshotJSON *string   `gorm:"column:output_snapshot_json"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Run) TableName() string { return "ai_runs" }

type RunEvent struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	RunID       int64     `gorm:"column:run_id"`
	Seq         uint64    `gorm:"column:seq"`
	EventID     string    `gorm:"column:event_id"`
	EventType   string    `gorm:"column:event_type"`
	DeltaText   string    `gorm:"column:delta_text"`
	PayloadJSON *string   `gorm:"column:payload_json"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (RunEvent) TableName() string { return "ai_run_events" }
