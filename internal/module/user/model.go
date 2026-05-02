package user

import "time"

type User struct {
	ID        int64     `gorm:"column:id"`
	RoleID    int64     `gorm:"column:role_id"`
	Username  string    `gorm:"column:username"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string {
	return "users"
}

type Profile struct {
	UserID int64  `gorm:"column:user_id"`
	Avatar string `gorm:"column:avatar"`
	IsDel  int    `gorm:"column:is_del"`
}

func (Profile) TableName() string {
	return "user_profiles"
}

type Role struct {
	ID    int64  `gorm:"column:id"`
	Name  string `gorm:"column:name"`
	IsDel int    `gorm:"column:is_del"`
}

func (Role) TableName() string {
	return "roles"
}

type QuickEntry struct {
	ID           int64 `gorm:"column:id" json:"id"`
	UserID       int64 `gorm:"column:user_id" json:"-"`
	PermissionID int64 `gorm:"column:permission_id" json:"permission_id"`
	Sort         int   `gorm:"column:sort" json:"sort"`
	IsDel        int   `gorm:"column:is_del" json:"-"`
}

func (QuickEntry) TableName() string {
	return "users_quick_entry"
}
