package aimessage

import "time"

type Conversation struct {
	ID     int64 `gorm:"column:id;primaryKey"`
	UserID int64 `gorm:"column:user_id"`
	IsDel  int   `gorm:"column:is_del"`
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
