package exporttask

import "time"

type Task struct {
	ID             int64      `gorm:"column:id"`
	UserID         int64      `gorm:"column:user_id"`
	Platform       string     `gorm:"column:platform"`
	Title          string     `gorm:"column:title"`
	Kind           string     `gorm:"column:kind"`
	FileName       string     `gorm:"column:file_name"`
	FileURL        string     `gorm:"column:file_url"`
	ObjectKey      string     `gorm:"column:object_key"`
	FileSize       *int64     `gorm:"column:file_size"`
	RowCount       *int64     `gorm:"column:row_count"`
	Status         int        `gorm:"column:status"`
	ClaimOwner     *string    `gorm:"column:claim_owner"`
	ClaimToken     uint64     `gorm:"column:claim_token"`
	ClaimExpiresAt *time.Time `gorm:"column:claim_expires_at"`
	ErrorMsg       string     `gorm:"column:error_msg"`
	ExpireAt       *time.Time `gorm:"column:expire_at"`
	IsDel          int        `gorm:"column:is_del"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (Task) TableName() string { return "export_tasks" }
