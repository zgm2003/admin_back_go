package modelpricing

import (
	"context"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
)

type HTTPService interface {
	PageInit(context.Context) (*PageInitResponse, *apperror.Error)
	List(context.Context, ListQuery) (*ListResponse, *apperror.Error)
	Detail(context.Context, string) (*ModelPriceDTO, *apperror.Error)
	SetOverride(context.Context, string, SetOverrideInput) (*MutationSummary, *apperror.Error)
	RestoreOfficial(context.Context, string, int64, uint64) (*MutationSummary, *apperror.Error)
}

type OptionDTO struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type PageInitDict struct {
	FamilyOptions []OptionDTO `json:"family_options"`
}

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type ListQuery struct {
	Family  string
	ModelID string
}

type ListResponse struct {
	List []ModelPriceDTO `json:"list"`
}

type RateDTO struct {
	Category  pricing.Category `json:"category"`
	Unit      string           `json:"unit"`
	TierKey   string           `json:"tier_key"`
	Price     string           `json:"price"`
	UnitScale int64            `json:"unit_scale"`
}

type PriceDTO struct {
	PricingVersion  string    `json:"pricing_version"`
	Source          string    `json:"source"`
	OverrideVersion uint64    `json:"override_version"`
	SourceURL       string    `json:"source_url"`
	VerifiedAt      string    `json:"verified_at"`
	Available       bool      `json:"available"`
	Rates           []RateDTO `json:"rates"`
}

type ModelPriceDTO struct {
	CatalogVendor              string   `json:"catalog_vendor"`
	ModelFamily                string   `json:"model_family"`
	ModelID                    string   `json:"model_id"`
	Aliases                    []string `json:"aliases"`
	PricingProfile             string   `json:"pricing_profile"`
	CatalogVersion             string   `json:"catalog_version"`
	MaxOutputTokens            int64    `json:"max_output_tokens"`
	ContextTierThresholdTokens int64    `json:"context_tier_threshold_tokens"`
	ReviewAfter                string   `json:"review_after"`
	Official                   PriceDTO `json:"official"`
	Effective                  PriceDTO `json:"effective"`
}

type MutationResponse struct {
	Before PriceDTO `json:"before"`
	After  PriceDTO `json:"after"`
}

var _ HTTPService = (*Service)(nil)
