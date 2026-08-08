package officialmodel

import (
	"context"

	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
)

type HTTPService interface {
	PageInit(context.Context) (*PageInitResponse, *apperror.Error)
	List(context.Context, ListQuery) (*ListResponse, *apperror.Error)
	Detail(context.Context, string) (*OfficialModelDTO, *apperror.Error)
	SetPriceOverride(context.Context, string, SetPriceOverrideInput) (*MutationSummary, *apperror.Error)
	RestoreOfficialPrice(context.Context, string, int64, uint64) (*MutationSummary, *apperror.Error)
}

type OptionDTO struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type PageInitDict struct {
	VendorOptions        []OptionDTO `json:"vendor_options"`
	FamilyOptions        []OptionDTO `json:"family_options"`
	LifecycleOptions     []OptionDTO `json:"lifecycle_options"`
	InputModalityOptions []OptionDTO `json:"input_modality_options"`
}

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type ListQuery struct {
	Vendor        string
	Family        string
	Lifecycle     LifecycleStatus
	InputModality string
	ModelID       string
}

type ListResponse struct {
	List []OfficialModelDTO `json:"list"`
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

type CapabilityDTO struct {
	InputModalities          []string              `json:"input_modalities"`
	OutputModalities         []string              `json:"output_modalities"`
	SupportsStreaming        bool                  `json:"supports_streaming"`
	SupportsTools            bool                  `json:"supports_tools"`
	SupportsStructuredOutput bool                  `json:"supports_structured_output"`
	SupportedParameters      []string              `json:"supported_parameters"`
	NativeFileInput          bool                  `json:"native_file_input"`
	ImageInput               *ImageInputCapability `json:"image_input"`
}

type OfficialModelDTO struct {
	CatalogVendor              string          `json:"catalog_vendor"`
	ModelFamily                string          `json:"model_family"`
	ModelID                    string          `json:"model_id"`
	ModelKind                  ModelKind       `json:"model_kind" validate:"oneof=chat embedding rerank image"`
	EmbeddingSpec              *EmbeddingSpec  `json:"embedding_spec"`
	Aliases                    []string        `json:"aliases"`
	LifecycleStatus            LifecycleStatus `json:"lifecycle_status"`
	CatalogVersion             string          `json:"catalog_version"`
	ContextWindowTokens        int64           `json:"context_window_tokens"`
	MaxOutputTokens            int64           `json:"max_output_tokens"`
	TokenCounterID             string          `json:"token_counter_id"`
	ContextTierThresholdTokens int64           `json:"context_tier_threshold_tokens"`
	Capabilities               CapabilityDTO   `json:"capabilities"`
	PricingProfile             string          `json:"pricing_profile"`
	Official                   PriceDTO        `json:"official"`
	Effective                  PriceDTO        `json:"effective"`
	ModelSourceURL             string          `json:"model_source_url"`
	PricingSourceURL           string          `json:"pricing_source_url"`
	RetrievedAt                string          `json:"retrieved_at"`
	ReviewAfter                string          `json:"review_after"`
}

type MutationResponse struct {
	Before PriceDTO `json:"before"`
	After  PriceDTO `json:"after"`
}

type RatePriceInput struct {
	Category  pricing.Category
	Unit      string
	TierKey   string
	UnitScale int64
	Price     string
}

type SetPriceOverrideInput struct {
	ExpectedVersion int64
	Prices          []RatePriceInput
	SourceURL       string
	VerifiedAt      string
	AdministratorID uint64
}

type MutationSummary struct {
	Before ResolvedModel
	After  ResolvedModel
}

var _ HTTPService = (*Service)(nil)
