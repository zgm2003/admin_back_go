package userloginlog

import "time"

type LoginLog struct {
	ID           int64     `gorm:"column:id"`
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

func (LoginLog) TableName() string {
	return "users_login_log"
}
