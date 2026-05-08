package userquickentry

import "time"

type EntryModel struct {
	ID           int64     `gorm:"column:id"`
	UserID       int64     `gorm:"column:user_id"`
	PermissionID int64     `gorm:"column:permission_id"`
	Sort         int       `gorm:"column:sort"`
	IsDel        int       `gorm:"column:is_del"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (EntryModel) TableName() string {
	return "users_quick_entry"
}
