package aiengine

import "time"

type Connection struct {
	ID            uint64     `gorm:"column:id;primaryKey"`
	Name          string     `gorm:"column:name"`
	EngineType    string     `gorm:"column:engine_type"`
	BaseURL       string     `gorm:"column:base_url"`
	APIKeyEnc     string     `gorm:"column:api_key_enc"`
	APIKeyHint    string     `gorm:"column:api_key_hint"`
	WorkspaceID   string     `gorm:"column:workspace_id"`
	ConfigJSON    string     `gorm:"column:config_json"`
	HealthStatus  string     `gorm:"column:health_status"`
	LastCheckedAt *time.Time `gorm:"column:last_checked_at"`
	Status        int        `gorm:"column:status"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedBy     uint64    `gorm:"column:created_by"`
	UpdatedBy     uint64    `gorm:"column:updated_by"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Connection) TableName() string { return "ai_engine_connections" }
