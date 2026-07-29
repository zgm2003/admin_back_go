package aiagent

import (
	"time"

	"admin_back_go/internal/module/ai/officialmodel"
)

type Agent struct {
	ID                   uint64    `gorm:"column:id;primaryKey"`
	ProviderID           uint64    `gorm:"column:provider_id"`
	Name                 string    `gorm:"column:name"`
	ModelID              string    `gorm:"column:model_id"`
	ModelDisplayName     string    `gorm:"column:model_display_name"`
	ScenesJSON           string    `gorm:"column:scenes_json"`
	SystemPrompt         string    `gorm:"column:system_prompt"`
	Avatar               string    `gorm:"column:avatar"`
	BillingMultiplierPPM int64     `gorm:"column:billing_multiplier_ppm"`
	Status               int       `gorm:"column:status"`
	IsDel                int       `gorm:"column:is_del"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (Agent) TableName() string { return "ai_agents" }

type Provider struct {
	ID           uint64 `gorm:"column:id;primaryKey"`
	Name         string `gorm:"column:name"`
	EngineType   string `gorm:"column:engine_type"`
	BaseURL      string `gorm:"column:base_url"`
	APIKeyEnc    string `gorm:"column:api_key_enc"`
	HealthStatus string `gorm:"column:health_status"`
	Status       int    `gorm:"column:status"`
	IsDel        int    `gorm:"column:is_del"`
}

func (Provider) TableName() string { return "ai_providers" }

type ProviderModel struct {
	ID                     uint64                      `gorm:"column:id;primaryKey"`
	ProviderID             uint64                      `gorm:"column:provider_id"`
	ModelID                string                      `gorm:"column:model_id"`
	DisplayName            string                      `gorm:"column:display_name"`
	OfficialModelID        *string                     `gorm:"column:official_model_id"`
	OfficialCatalogVersion *string                     `gorm:"column:official_catalog_version"`
	MappingStatus          officialmodel.MappingStatus `gorm:"column:mapping_status"`
	MappedAt               *time.Time                  `gorm:"column:mapped_at"`
	Status                 int                         `gorm:"column:status"`
	CreatedAt              time.Time                   `gorm:"column:created_at"`
	UpdatedAt              time.Time                   `gorm:"column:updated_at"`
}

func (ProviderModel) TableName() string { return "ai_provider_models" }
