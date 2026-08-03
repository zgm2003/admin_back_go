package aiprovider

import (
	"fmt"
	"time"

	"admin_back_go/internal/module/ai/officialmodel"
)

type ModelKind string

const (
	ModelKindChat      ModelKind = "chat"
	ModelKindEmbedding ModelKind = "embedding"
	ModelKindRerank    ModelKind = "rerank"
)

func (kind ModelKind) Validate() error {
	switch kind {
	case ModelKindChat, ModelKindEmbedding, ModelKindRerank:
		return nil
	default:
		return fmt.Errorf("invalid provider model kind %q", kind)
	}
}

type Provider struct {
	ID                  uint64     `gorm:"column:id;primaryKey"`
	Name                string     `gorm:"column:name"`
	EngineType          string     `gorm:"column:engine_type"`
	BaseURL             string     `gorm:"column:base_url"`
	APIProtocol         string     `gorm:"column:api_protocol"`
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
	ID                     uint64                      `gorm:"column:id;primaryKey"`
	ProviderID             uint64                      `gorm:"column:provider_id"`
	ModelID                string                      `gorm:"column:model_id"`
	ModelKind              ModelKind                   `gorm:"column:model_kind"`
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
