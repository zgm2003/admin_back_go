package aiconversation

import "time"

type Conversation struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	UserID        int64      `gorm:"column:user_id"`
	AgentID       int64      `gorm:"column:agent_id"`
	Title         string     `gorm:"column:title"`
	LastMessageAt *time.Time `gorm:"column:last_message_at"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Conversation) TableName() string { return "ai_conversations" }
