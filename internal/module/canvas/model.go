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
