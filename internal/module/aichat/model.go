package aichat

import "time"

type Conversation struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	UserID        int64      `gorm:"column:user_id"`
	AgentID       int64      `gorm:"column:agent_id"`
	Title         string     `gorm:"column:title"`
	LastMessageAt *time.Time `gorm:"column:last_message_at"`
	Status        int        `gorm:"column:status"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Conversation) TableName() string { return "ai_conversations" }

type Message struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	ConversationID int64     `gorm:"column:conversation_id"`
	Role           int       `gorm:"column:role"`
	Content        string    `gorm:"column:content"`
	MetaJSON       *string   `gorm:"column:meta_json"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (Message) TableName() string { return "ai_messages" }

type Run struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	RequestID          string    `gorm:"column:request_id"`
	UserID             int64     `gorm:"column:user_id"`
	AgentID            int64     `gorm:"column:agent_id"`
	ConversationID     int64     `gorm:"column:conversation_id"`
	UserMessageID      *int64    `gorm:"column:user_message_id"`
	AssistantMessageID *int64    `gorm:"column:assistant_message_id"`
	RunStatus          int       `gorm:"column:run_status"`
	ErrorMsg           *string   `gorm:"column:error_msg"`
	PromptTokens       *int      `gorm:"column:prompt_tokens"`
	CompletionTokens   *int      `gorm:"column:completion_tokens"`
	TotalTokens        *int      `gorm:"column:total_tokens"`
	LatencyMS          *int      `gorm:"column:latency_ms"`
	ModelSnapshot      *string   `gorm:"column:model_snapshot"`
	MetaJSON           *string   `gorm:"column:meta_json"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Run) TableName() string { return "ai_runs" }

type Step struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	RunID         int64     `gorm:"column:run_id"`
	StepNo        int       `gorm:"column:step_no"`
	StepType      int       `gorm:"column:step_type"`
	AgentID       int64     `gorm:"column:agent_id"`
	ModelSnapshot *string   `gorm:"column:model_snapshot"`
	Status        int       `gorm:"column:status"`
	ErrorMsg      *string   `gorm:"column:error_msg"`
	LatencyMS     *int      `gorm:"column:latency_ms"`
	PayloadJSON   *string   `gorm:"column:payload_json"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Step) TableName() string { return "ai_run_steps" }
