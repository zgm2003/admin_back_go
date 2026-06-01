package canvas

import "time"

const (
	StatusEnabled  = 1
	StatusDisabled = 2
	IsDelDeleted   = 1
	IsDelActive    = 2
	AssetTypeText  = "text"
	AssetTypeImage = "image"
)

type Prompt struct {
	ID        int64     `gorm:"column:id"`
	Slug      string    `gorm:"column:slug"`
	Category  string    `gorm:"column:category"`
	Title     string    `gorm:"column:title"`
	CoverURL  string    `gorm:"column:cover_url"`
	Prompt    string    `gorm:"column:prompt"`
	Preview   string    `gorm:"column:preview"`
	TagsJSON  string    `gorm:"column:tags_json"`
	SourceURL string    `gorm:"column:source_url"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Prompt) TableName() string { return "canvas_prompts" }

type Asset struct {
	ID          int64     `gorm:"column:id"`
	Slug        string    `gorm:"column:slug"`
	Type        string    `gorm:"column:type"`
	Category    string    `gorm:"column:category"`
	Title       string    `gorm:"column:title"`
	CoverURL    string    `gorm:"column:cover_url"`
	Description string    `gorm:"column:description"`
	Content     string    `gorm:"column:content"`
	URL         string    `gorm:"column:url"`
	TagsJSON    string    `gorm:"column:tags_json"`
	Status      int       `gorm:"column:status"`
	IsDel       int       `gorm:"column:is_del"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Asset) TableName() string { return "canvas_assets" }

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
	Status          string     `gorm:"column:status"`
	ErrorMessage    string     `gorm:"column:error_message"`
	IsDel           int        `gorm:"column:is_del"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
}

func (VideoTask) TableName() string { return "canvas_video_tasks" }
