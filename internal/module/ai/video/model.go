package aivideo

import "time"

const (
	IsDelDeleted = 1
	IsDelActive  = 2
)

type VideoTask struct {
	ID              int64      `gorm:"column:id"`
	UserID          int64      `gorm:"column:user_id"`
	AgentID         int64      `gorm:"column:agent_id"`
	ProviderID      int64      `gorm:"column:provider_id"`
	ModelID         string     `gorm:"column:model_id"`
	Prompt          string     `gorm:"column:prompt"`
	DurationSeconds int        `gorm:"column:duration_seconds"`
	Size            string     `gorm:"column:size"`
	ResolutionName  string     `gorm:"column:resolution_name"`
	ProviderTaskID  string     `gorm:"column:provider_task_id"`
	RunID           int64      `gorm:"column:run_id"`
	Status          string     `gorm:"column:status"`
	ErrorMessage    string     `gorm:"column:error_message"`
	IsDel           int        `gorm:"column:is_del"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
}

func (VideoTask) TableName() string { return "canvas_video_tasks" }
