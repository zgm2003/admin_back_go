package exporttask

import "time"

type Task struct {
	ID        int64      `gorm:"column:id"`
	UserID    int64      `gorm:"column:user_id"`
	Title     string     `gorm:"column:title"`
	FileName  string     `gorm:"column:file_name"`
	FileURL   string     `gorm:"column:file_url"`
	FileSize  *int64     `gorm:"column:file_size"`
	RowCount  *int64     `gorm:"column:row_count"`
	Status    int        `gorm:"column:status"`
	ErrorMsg  string     `gorm:"column:error_msg"`
	ExpireAt  *time.Time `gorm:"column:expire_at"`
	IsDel     int        `gorm:"column:is_del"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (Task) TableName() string { return "export_tasks" }
