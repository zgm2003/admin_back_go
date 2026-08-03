package contextengine

import (
	"context"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

type memoryModelLimits struct {
	KnownInputBudget uint64
	MaxOutputTokens  uint64
	TokenCounterID   string
}

func loadMemoryModelLimits(ctx context.Context, db *gorm.DB, providerModelID uint64) (memoryModelLimits, error) {
	if db == nil || providerModelID == 0 || officialmodel.Default == nil {
		return memoryModelLimits{}, infraai.ErrInvalidConfig
	}
	var row struct {
		ModelKind              string  `gorm:"column:model_kind"`
		ModelStatus            int     `gorm:"column:model_status"`
		OfficialModelID        *string `gorm:"column:official_model_id"`
		OfficialCatalogVersion *string `gorm:"column:official_catalog_version"`
		MappingStatus          string  `gorm:"column:mapping_status"`
	}
	if err := db.WithContext(ctx).Table("ai_provider_models AS pm").
		Select("pm.model_kind, pm.status AS model_status, pm.official_model_id, pm.official_catalog_version, pm.mapping_status").
		Where("pm.id = ?", providerModelID).Take(&row).Error; err != nil {
		return memoryModelLimits{}, err
	}
	if row.ModelKind != string(aiprovider.ModelKindChat) || row.ModelStatus != enum.CommonYes || row.OfficialModelID == nil ||
		row.OfficialCatalogVersion == nil || row.MappingStatus != string(officialmodel.MappingStatusMapped) {
		return memoryModelLimits{}, infraai.ErrInvalidConfig
	}
	model, err := officialmodel.Default.ResolveIdentity(*row.OfficialModelID)
	if err != nil || model.CatalogVersion != *row.OfficialCatalogVersion || model.LifecycleStatus != officialmodel.LifecycleActive ||
		model.ContextWindowTokens <= model.MaxOutputTokens || model.MaxOutputTokens <= 0 {
		return memoryModelLimits{}, infraai.ErrInvalidConfig
	}
	if _, err := infraai.ResolveTokenCounter(model.TokenCounterID); err != nil {
		return memoryModelLimits{}, err
	}
	return memoryModelLimits{KnownInputBudget: uint64(model.ContextWindowTokens - model.MaxOutputTokens),
		MaxOutputTokens: uint64(model.MaxOutputTokens), TokenCounterID: model.TokenCounterID}, nil
}
