package role

import "time"

type Role struct {
	ID        int64     `gorm:"column:id"`
	Name      string    `gorm:"column:name"`
	IsDefault int       `gorm:"column:is_default"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Role) TableName() string {
	return "roles"
}

type RolePermission struct {
	ID           int64 `gorm:"column:id"`
	RoleID       int64 `gorm:"column:role_id"`
	PermissionID int64 `gorm:"column:permission_id"`
	IsDel        int   `gorm:"column:is_del"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

type User struct {
	ID     int64 `gorm:"column:id"`
	RoleID int64 `gorm:"column:role_id"`
	IsDel  int   `gorm:"column:is_del"`
}

func (User) TableName() string {
	return "users"
}
