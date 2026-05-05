package notificationtask

import "time"

type Task struct {
	ID         int64      `gorm:"column:id"`
	Title      string     `gorm:"column:title"`
	Content    string     `gorm:"column:content"`
	Type       int        `gorm:"column:type"`
	Level      int        `gorm:"column:level"`
	Link       string     `gorm:"column:link"`
	Platform   string     `gorm:"column:platform"`
	TargetType int        `gorm:"column:target_type"`
	TargetIDs  string     `gorm:"column:target_ids"`
	Status     int        `gorm:"column:status"`
	TotalCount int        `gorm:"column:total_count"`
	SentCount  int        `gorm:"column:sent_count"`
	SendAt     *time.Time `gorm:"column:send_at"`
	ErrorMsg   string     `gorm:"column:error_msg"`
	CreatedBy  int64      `gorm:"column:created_by"`
	IsDel      int        `gorm:"column:is_del"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (Task) TableName() string {
	return "notification_task"
}

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
