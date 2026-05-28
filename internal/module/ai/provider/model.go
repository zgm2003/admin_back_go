package aiprovider

import "time"

type Provider struct {
	ID                  uint64     `gorm:"column:id;primaryKey"`
	Name                string     `gorm:"column:name"`
	EngineType          string     `gorm:"column:engine_type"`
	BaseURL             string     `gorm:"column:base_url"`
	APIKeyEnc           string     `gorm:"column:api_key_enc"`
	APIKeyHint          string     `gorm:"column:api_key_hint"`
	HealthStatus        string     `gorm:"column:health_status"`
	LastCheckedAt       *time.Time `gorm:"column:last_checked_at"`
	LastCheckError      string     `gorm:"column:last_check_error"`
	LastModelSyncAt     *time.Time `gorm:"column:last_model_sync_at"`
	LastModelSyncStatus string     `gorm:"column:last_model_sync_status"`
	LastModelSyncError  string     `gorm:"column:last_model_sync_error"`
	Status              int        `gorm:"column:status"`
	IsDel               int        `gorm:"column:is_del"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (Provider) TableName() string { return "ai_providers" }

type ProviderModel struct {
	ID          uint64    `gorm:"column:id;primaryKey"`
	ProviderID  uint64    `gorm:"column:provider_id"`
	ModelID     string    `gorm:"column:model_id"`
	DisplayName string    `gorm:"column:display_name"`
	Status      int       `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (ProviderModel) TableName() string { return "ai_provider_models" }
