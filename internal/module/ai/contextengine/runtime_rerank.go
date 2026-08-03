package contextengine

import (
	"context"
	"errors"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

const (
	runtimeRerankMaxDocuments   uint32 = 50
	runtimeRerankMaxInputTokens int64  = 100_000
)

type GormRerankResolver struct {
	db      *gorm.DB
	factory infraai.RerankFactory
	box     secretbox.Box
}

func NewRerankResolver(client *database.Client, factory infraai.RerankFactory, box secretbox.Box) *GormRerankResolver {
	if client == nil || client.Gorm == nil || factory == nil {
		return nil
	}
	return &GormRerankResolver{db: client.Gorm, factory: factory, box: box}
}

func (resolver *GormRerankResolver) ResolveRerank(ctx context.Context, profile ContextProfile) (infraai.RerankClient, error) {
	if resolver == nil || resolver.db == nil || resolver.factory == nil || profile.RerankerProviderModelID == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var row struct {
		ModelID         string `gorm:"column:model_id"`
		ModelKind       string `gorm:"column:model_kind"`
		ModelStatus     int    `gorm:"column:model_status"`
		EngineType      string `gorm:"column:engine_type"`
		BaseURL         string `gorm:"column:base_url"`
		APIKeyEnc       string `gorm:"column:api_key_enc"`
		ProviderStatus  int    `gorm:"column:provider_status"`
		ProviderDeleted int    `gorm:"column:provider_deleted"`
	}
	err := resolver.db.WithContext(ctx).Table("ai_provider_models AS model").
		Select("model.model_id, model.model_kind, model.status AS model_status, provider.engine_type, provider.base_url, provider.api_key_enc, provider.status AS provider_status, provider.is_del AS provider_deleted").
		Joins("JOIN ai_providers AS provider ON provider.id = model.provider_id").
		Where("model.id = ?", *profile.RerankerProviderModelID).Take(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ModelKind != string(aiprovider.ModelKindRerank) || row.ModelStatus != enum.CommonYes || row.ProviderStatus != enum.CommonYes || row.ProviderDeleted != enum.CommonNo {
		return nil, errors.New("rerank provider model is not enabled")
	}
	apiKey, err := resolver.box.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, err
	}
	return resolver.factory.NewRerankClient(ctx, infraai.RerankClientConfig{EngineType: infraai.EngineType(row.EngineType), ModelKind: row.ModelKind,
		ModelID: row.ModelID, BaseURL: row.BaseURL, APIKey: apiKey, Capabilities: infraai.RerankCapabilities{
			MaxDocuments: runtimeRerankMaxDocuments, MaxInputTokens: runtimeRerankMaxInputTokens, TokenCounterID: profile.EmbeddingTokenCounterID}})
}

var _ RuntimeRerankResolver = (*GormRerankResolver)(nil)
