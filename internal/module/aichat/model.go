package aichat

import "time"

type Conversation struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	AgentID       uint64     `gorm:"column:agent_id"`
	UserID        int64      `gorm:"column:user_id"`
	Title         string     `gorm:"column:title"`
	LastMessageAt *time.Time `gorm:"column:last_message_at"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Conversation) TableName() string { return "ai_conversations" }

type Message struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	ConversationID int64     `gorm:"column:conversation_id"`
	Role           int       `gorm:"column:role"`
	ContentType    string    `gorm:"column:content_type"`
	Content        string    `gorm:"column:content"`
	MetaJSON       *string   `gorm:"column:meta_json"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (Message) TableName() string { return "ai_messages" }

type Run struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	RunStatus   int        `gorm:"column:run_status"`
	ErrorMsg    *string    `gorm:"column:error_msg"`
	CompletedAt *time.Time `gorm:"column:completed_at"`
	IsDel       int        `gorm:"column:is_del"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (Run) TableName() string { return "ai_runs" }

type Agent struct {
	ID               uint64 `gorm:"column:id;primaryKey"`
	ProviderID       uint64 `gorm:"column:provider_id"`
	Name             string `gorm:"column:name"`
	ModelID          string `gorm:"column:model_id"`
	ModelDisplayName string `gorm:"column:model_display_name"`
	ScenesJSON       string `gorm:"column:scenes_json"`
	SystemPrompt     string `gorm:"column:system_prompt"`
	Status           int    `gorm:"column:status"`
	IsDel            int    `gorm:"column:is_del"`
}

func (Agent) TableName() string { return "ai_agents" }

type Provider struct {
	ID         uint64 `gorm:"column:id;primaryKey"`
	EngineType string `gorm:"column:engine_type"`
	BaseURL    string `gorm:"column:base_url"`
	APIKeyEnc  string `gorm:"column:api_key_enc"`
	Status     int    `gorm:"column:status"`
	IsDel      int    `gorm:"column:is_del"`
}

func (Provider) TableName() string { return "ai_providers" }
