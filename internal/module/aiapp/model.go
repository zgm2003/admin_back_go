package aiapp

import "time"

type App struct {
	ID                  uint64    `gorm:"column:id;primaryKey"`
	EngineConnectionID  uint64    `gorm:"column:engine_connection_id"`
	Name                string    `gorm:"column:name"`
	Code                string    `gorm:"column:code"`
	AppType             string    `gorm:"column:app_type"`
	EngineAppID         string    `gorm:"column:engine_app_id"`
	EngineAppAPIKeyEnc  string    `gorm:"column:engine_app_api_key_enc"`
	EngineAppAPIKeyHint string    `gorm:"column:engine_app_api_key_hint"`
	DefaultResponseMode string    `gorm:"column:default_response_mode"`
	RuntimeConfigJSON   string    `gorm:"column:runtime_config_json"`
	ModelSnapshotJSON   string    `gorm:"column:model_snapshot_json"`
	Status              int       `gorm:"column:status"`
	IsDel               int       `gorm:"column:is_del"`
	CreatedBy           uint64    `gorm:"column:created_by"`
	UpdatedBy           uint64    `gorm:"column:updated_by"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (App) TableName() string { return "ai_apps" }

type Binding struct {
	ID        uint64    `gorm:"column:id;primaryKey"`
	AppID     uint64    `gorm:"column:app_id"`
	BindType  string    `gorm:"column:bind_type"`
	BindKey   string    `gorm:"column:bind_key"`
	Sort      int       `gorm:"column:sort"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Binding) TableName() string { return "ai_app_bindings" }

type EngineConnection struct {
	ID           uint64 `gorm:"column:id;primaryKey"`
	Name         string `gorm:"column:name"`
	EngineType   string `gorm:"column:engine_type"`
	BaseURL      string `gorm:"column:base_url"`
	APIKeyEnc    string `gorm:"column:api_key_enc"`
	HealthStatus string `gorm:"column:health_status"`
	Status       int    `gorm:"column:status"`
	IsDel        int    `gorm:"column:is_del"`
}

func (EngineConnection) TableName() string { return "ai_engine_connections" }
