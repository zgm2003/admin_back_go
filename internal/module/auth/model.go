package auth

import "time"

const (
	commonYes = 1
	commonNo  = 2
)

type UserCredential struct {
	ID           int64  `gorm:"column:id"`
	Email        string `gorm:"column:email"`
	Phone        string `gorm:"column:phone"`
	PasswordHash string `gorm:"column:password"`
	Status       int    `gorm:"column:status"`
	IsDel        int    `gorm:"column:is_del"`
}

func (UserCredential) TableName() string {
	return "users"
}

type DefaultRole struct {
	ID int64 `gorm:"column:id"`
}

func (DefaultRole) TableName() string {
	return "roles"
}

type CreateUserInput struct {
	Username string
	RoleID   int64
	Email    *string
	Phone    *string
}

type CreateProfileInput struct {
	UserID int64
	Sex    int
}

type userCreateRow struct {
	ID        int64     `gorm:"column:id"`
	RoleID    int64     `gorm:"column:role_id"`
	Username  string    `gorm:"column:username"`
	Email     *string   `gorm:"column:email"`
	Phone     *string   `gorm:"column:phone"`
	Password  *string   `gorm:"column:password"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (userCreateRow) TableName() string {
	return "users"
}

type LoginAttempt struct {
	UserID       *int64
	LoginAccount string
	LoginType    string
	Platform     string
	IP           string
	UserAgent    string
	IsSuccess    int
	Reason       string
}

type loginAttemptRow struct {
	UserID       *int64    `gorm:"column:user_id"`
	LoginAccount string    `gorm:"column:login_account"`
	LoginType    string    `gorm:"column:login_type"`
	Platform     string    `gorm:"column:platform"`
	IP           string    `gorm:"column:ip"`
	UserAgent    string    `gorm:"column:ua"`
	IsSuccess    int       `gorm:"column:is_success"`
	Reason       string    `gorm:"column:reason"`
	IsDel        int       `gorm:"column:is_del"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (loginAttemptRow) TableName() string {
	return "users_login_log"
}
