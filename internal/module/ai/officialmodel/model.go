package officialmodel

import (
	"time"

	"admin_back_go/internal/module/ai/pricing"
)

type PriceOverride struct {
	ID            uint64              `gorm:"column:id;primaryKey"`
	CatalogVendor string              `gorm:"column:catalog_vendor"`
	ModelID       string              `gorm:"column:model_id"`
	Version       uint64              `gorm:"column:version"`
	SourceURL     string              `gorm:"column:source_url"`
	VerifiedAt    time.Time           `gorm:"column:verified_at"`
	UpdatedBy     uint64              `gorm:"column:updated_by"`
	CreatedAt     time.Time           `gorm:"column:created_at"`
	UpdatedAt     time.Time           `gorm:"column:updated_at"`
	Rates         []PriceOverrideRate `gorm:"foreignKey:OverrideID"`
}

func (PriceOverride) TableName() string { return "ai_official_model_price_overrides" }

type PriceOverrideRate struct {
	ID         uint64           `gorm:"column:id;primaryKey"`
	OverrideID uint64           `gorm:"column:override_id"`
	Category   pricing.Category `gorm:"column:category"`
	Unit       string           `gorm:"column:unit"`
	TierKey    string           `gorm:"column:tier_key"`
	PriceUnits int64            `gorm:"column:price_units"`
	UnitScale  int64            `gorm:"column:unit_scale"`
}

func (PriceOverrideRate) TableName() string {
	return "ai_official_model_price_override_rates"
}
