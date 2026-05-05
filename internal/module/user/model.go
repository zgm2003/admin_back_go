package user

import "time"

type User struct {
	ID        int64     `gorm:"column:id"`
	RoleID    int64     `gorm:"column:role_id"`
	Username  string    `gorm:"column:username"`
	Email     string    `gorm:"column:email"`
	Phone     string    `gorm:"column:phone"`
	Password  *string   `gorm:"column:password"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string {
	return "users"
}

type Profile struct {
	UserID        int64      `gorm:"column:user_id"`
	Avatar        string     `gorm:"column:avatar"`
	Bio           string     `gorm:"column:bio"`
	Sex           int        `gorm:"column:sex"`
	Birthday      *time.Time `gorm:"column:birthday"`
	AddressID     int64      `gorm:"column:address_id"`
	DetailAddress string     `gorm:"column:detail_address"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (Profile) TableName() string {
	return "user_profiles"
}

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

type Address struct {
	ID        int64     `gorm:"column:id"`
	ParentID  int64     `gorm:"column:parent_id"`
	Code      string    `gorm:"column:code"`
	Name      string    `gorm:"column:name"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Address) TableName() string {
	return "address"
}
