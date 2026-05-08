package aiprompt

import "time"

type Prompt struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	UserID     int64     `gorm:"column:user_id"`
	Title      string    `gorm:"column:title"`
	Content    string    `gorm:"column:content"`
	Category   string    `gorm:"column:category"`
	Tags       string    `gorm:"column:tags"`
	Variables  *string   `gorm:"column:variables"`
	IsFavorite int       `gorm:"column:is_favorite"`
	UseCount   int64     `gorm:"column:use_count"`
	Sort       int       `gorm:"column:sort"`
	IsDel      int       `gorm:"column:is_del"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (Prompt) TableName() string {
	return "ai_prompts"
}
