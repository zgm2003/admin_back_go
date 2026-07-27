package airun

import "time"

type Run struct {
	ID                    int64      `gorm:"column:id;primaryKey"`
	Platform              string     `gorm:"column:platform"`
	ConversationID        *int64     `gorm:"column:conversation_id"`
	RequestID             string     `gorm:"column:request_id"`
	RequestFingerprint    []byte     `gorm:"column:request_fingerprint"`
	RequestIdentityStatus string     `gorm:"column:request_identity_status"`
	RequestIdentityMarker string     `gorm:"column:request_identity_marker"`
	IdempotencyKey        *string    `gorm:"column:idempotency_key"`
	UserMessageID         *int64     `gorm:"column:user_message_id"`
	AssistantMessageID    *int64     `gorm:"column:assistant_message_id"`
	UserID                int64      `gorm:"column:user_id"`
	AgentID               int64      `gorm:"column:agent_id"`
	ProviderID            int64      `gorm:"column:provider_id"`
	ModelID               string     `gorm:"column:model_id"`
	ModelDisplayName      string     `gorm:"column:model_display_name"`
	InputSnapshot         string     `gorm:"column:input_snapshot"`
	PricingSnapshotJSON   string     `gorm:"column:pricing_snapshot_json"`
	Status                string     `gorm:"column:status"`
	BillingStatus         string     `gorm:"column:billing_status"`
	BillingReason         string     `gorm:"column:billing_reason"`
	PromptTokens          uint       `gorm:"column:prompt_tokens"`
	CompletionTokens      uint       `gorm:"column:completion_tokens"`
	TotalTokens           uint       `gorm:"column:total_tokens"`
	DurationMS            *uint      `gorm:"column:duration_ms"`
	ErrorMessage          string     `gorm:"column:error_message"`
	StartedAt             *time.Time `gorm:"column:started_at"`
	FinishedAt            *time.Time `gorm:"column:finished_at"`
	LikedAt               *time.Time `gorm:"column:liked_at"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (Run) TableName() string { return "ai_runs" }

type RunEvent struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	RunID     int64     `gorm:"column:run_id"`
	Seq       uint      `gorm:"column:seq"`
	EventType string    `gorm:"column:event_type"`
	Message   string    `gorm:"column:message"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (RunEvent) TableName() string { return "ai_run_events" }
