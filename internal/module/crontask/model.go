package crontask

import "time"

const (
	CommonYes = 1
	CommonNo  = 2

	LogStatusSuccess = 1
	LogStatusFailed  = 2
	LogStatusRunning = 3

	RegistryStatusRegistered  = "registered"
	RegistryStatusMissing     = "missing"
	RegistryStatusDisabled    = "disabled"
	RegistryStatusInvalidCron = "invalid_cron"
)

type Task struct {
	ID           int64     `gorm:"column:id"`
	Name         string    `gorm:"column:name"`
	Title        string    `gorm:"column:title"`
	Description  string    `gorm:"column:description"`
	Cron         string    `gorm:"column:cron"`
	CronReadable string    `gorm:"column:cron_readable"`
	Handler      string    `gorm:"column:handler"`
	Status       int       `gorm:"column:status"`
	IsDel        int       `gorm:"column:is_del"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Task) TableName() string { return "cron_task" }

type TaskLog struct {
	ID         int64      `gorm:"column:id"`
	TaskID     int64      `gorm:"column:task_id"`
	TaskName   string     `gorm:"column:task_name"`
	StartTime  *time.Time `gorm:"column:start_time"`
	EndTime    *time.Time `gorm:"column:end_time"`
	DurationMS *int64     `gorm:"column:duration_ms"`
	Status     int        `gorm:"column:status"`
	Result     string     `gorm:"column:result"`
	ErrorMsg   string     `gorm:"column:error_msg"`
	IsDel      int        `gorm:"column:is_del"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (TaskLog) TableName() string { return "cron_task_log" }
