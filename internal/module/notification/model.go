package notification

import "time"

type Notification struct {
	ID        int64     `gorm:"column:id"`
	UserID    int64     `gorm:"column:user_id"`
	Title     string    `gorm:"column:title"`
	Content   string    `gorm:"column:content"`
	Type      int       `gorm:"column:type"`
	Level     int       `gorm:"column:level"`
	Link      string    `gorm:"column:link"`
	Platform  string    `gorm:"column:platform"`
	IsRead    int       `gorm:"column:is_read"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Notification) TableName() string {
	return "notifications"
}
