package operationlog

import "time"

const (
	CommonYes = 1
	CommonNo  = 2
)

type Log struct {
	ID           int64     `gorm:"column:id"`
	UserID       int64     `gorm:"column:user_id"`
	Action       string    `gorm:"column:action"`
	RequestData  string    `gorm:"column:request_data"`
	ResponseData string    `gorm:"column:response_data"`
	IsDel        int       `gorm:"column:is_del"`
	IsSuccess    int       `gorm:"column:is_success"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Log) TableName() string {
	return "operation_logs"
}

type User struct {
	ID    int64  `gorm:"column:id"`
	Name  string `gorm:"column:username"`
	Email string `gorm:"column:email"`
}

func (User) TableName() string {
	return "users"
}
