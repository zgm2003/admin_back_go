package aiagent

import (
	"context"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	SceneArr                 []dict.Option[string] `json:"scene_arr"`
	CommonStatusArr          []dict.Option[int]    `json:"common_status_arr"`
	ProviderOptions          []EngineOption        `json:"provider_options"`
	ModelOptions             []ModelOption         `json:"provider_model_options"`
	BillingMultiplierDefault string                `json:"billing_multiplier_default"`
}

type EngineOption struct {
	Label      string `json:"label"`
	Value      uint64 `json:"value"`
	EngineType string `json:"engine_type"`
}

type ModelOption struct {
	Label                      string                    `json:"label"`
	Value                      string                    `json:"value"`
	ProviderID                 uint64                    `json:"provider_id"`
	ModelID                    string                    `json:"model_id"`
	DisplayName                string                    `json:"display_name"`
	BillingMultiplier          string                    `json:"billing_multiplier"`
	OfficialModel              *OfficialModelSummaryDTO  `json:"official_model,omitempty"`
	Capabilities               *EffectiveCapabilitiesDTO `json:"capabilities,omitempty"`
	PricingVersion             string                    `json:"pricing_version,omitempty"`
	CatalogVersion             string                    `json:"catalog_version,omitempty"`
	CatalogVendor              string                    `json:"catalog_vendor,omitempty"`
	CatalogModelID             string                    `json:"catalog_model_id,omitempty"`
	PriceSource                string                    `json:"price_source,omitempty"`
	OverrideVersion            uint64                    `json:"override_version"`
	PriceSourceURL             string                    `json:"price_source_url,omitempty"`
	PriceVerifiedAt            string                    `json:"price_verified_at,omitempty"`
	ContextTierThresholdTokens int64                     `json:"context_tier_threshold_tokens"`
	CatalogRates               []CatalogRateDTO          `json:"catalog_rates,omitempty"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Scene       string
	ProviderID  uint64
	Status      *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []AgentDTO `json:"list"`
	Page Page       `json:"page"`
}

type DetailResponse struct {
	AgentDTO
}

type AgentDTO struct {
	ID                         uint64                    `json:"id"`
	ProviderID                 uint64                    `json:"provider_id"`
	ProviderName               string                    `json:"provider_name"`
	EngineType                 string                    `json:"engine_type"`
	Name                       string                    `json:"name"`
	ModelID                    string                    `json:"model_id"`
	ModelDisplayName           string                    `json:"model_display_name"`
	Scenes                     []string                  `json:"scenes"`
	SceneNames                 []string                  `json:"scene_names"`
	SystemPrompt               string                    `json:"system_prompt"`
	Avatar                     string                    `json:"avatar"`
	Status                     int                       `json:"status"`
	StatusName                 string                    `json:"status_name"`
	CreatedAt                  string                    `json:"created_at"`
	UpdatedAt                  string                    `json:"updated_at"`
	BillingMultiplier          string                    `json:"billing_multiplier"`
	OfficialModel              *OfficialModelSummaryDTO  `json:"official_model,omitempty"`
	Capabilities               *EffectiveCapabilitiesDTO `json:"capabilities,omitempty"`
	ProviderModelID            uint64                    `json:"provider_model_id"`
	PricingVersion             string                    `json:"pricing_version,omitempty"`
	CatalogVersion             string                    `json:"catalog_version,omitempty"`
	CatalogVendor              string                    `json:"catalog_vendor,omitempty"`
	CatalogModelID             string                    `json:"catalog_model_id,omitempty"`
	PriceSource                string                    `json:"price_source,omitempty"`
	OverrideVersion            uint64                    `json:"override_version"`
	PriceSourceURL             string                    `json:"price_source_url,omitempty"`
	PriceVerifiedAt            string                    `json:"price_verified_at,omitempty"`
	ContextTierThresholdTokens int64                     `json:"context_tier_threshold_tokens"`
	CatalogRates               []CatalogRateDTO          `json:"catalog_rates,omitempty"`
}

type CatalogRateDTO struct {
	Category  string `json:"category"`
	Unit      string `json:"unit"`
	TierKey   string `json:"tier_key"`
	Price     string `json:"price"`
	UnitScale int64  `json:"unit_scale"`
}

type OfficialModelSummaryDTO struct {
	ModelID             string                        `json:"model_id"`
	CatalogVersion      string                        `json:"catalog_version"`
	CatalogVendor       string                        `json:"catalog_vendor"`
	ModelFamily         string                        `json:"model_family"`
	LifecycleStatus     officialmodel.LifecycleStatus `json:"lifecycle_status"`
	ContextWindowTokens int64                         `json:"context_window_tokens"`
	MaxOutputTokens     int64                         `json:"max_output_tokens"`
}

type EffectiveCapabilitiesDTO struct {
	InputModalities          []string                     `json:"input_modalities"`
	OutputModalities         []string                     `json:"output_modalities"`
	SupportsTools            bool                         `json:"supports_tools"`
	SupportsStreaming        bool                         `json:"supports_streaming"`
	SupportsStructuredOutput bool                         `json:"supports_structured_output"`
	RuntimeParameters        RuntimeParameterCapabilities `json:"runtime_parameters"`
	Attachments              AttachmentCapabilities       `json:"attachments"`
}

type RuntimeParameterCapabilities struct {
	Temperature TemperatureParameterCapability `json:"temperature"`
	MaxHistory  MaxHistoryParameterCapability  `json:"max_history"`
}

type TemperatureParameterCapability struct {
	Supported bool    `json:"supported"`
	Default   float64 `json:"default"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
}

type MaxHistoryParameterCapability struct {
	Supported    bool `json:"supported"`
	Default      int  `json:"default"`
	Min          int  `json:"min"`
	Max          int  `json:"max"`
	Transitional bool `json:"transitional"`
}

type AttachmentCapabilities struct {
	Image      ImageAttachmentCapability      `json:"image"`
	NativeFile NativeFileAttachmentCapability `json:"native_file"`
}

type ImageAttachmentCapability struct {
	Enabled      bool     `json:"enabled"`
	MIMETypes    []string `json:"mime_types"`
	MaxFiles     int      `json:"max_files"`
	MaxFileBytes int64    `json:"max_file_bytes"`
}

type NativeFileAttachmentCapability struct {
	Enabled bool `json:"enabled"`
}

type CreateInput struct {
	ProviderID        uint64
	Name              string
	ModelID           string
	Scenes            []string
	SystemPrompt      string
	Avatar            string
	Status            int
	BillingMultiplier string
}

type UpdateInput = CreateInput

type OptionQuery struct {
	UserID int64
	Scene  string
}

type AgentOption struct {
	ID              uint64                    `json:"id"`
	Name            string                    `json:"name"`
	Avatar          string                    `json:"avatar"`
	SystemPrompt    string                    `json:"system_prompt"`
	ProviderModelID uint64                    `json:"provider_model_id"`
	OfficialModel   *OfficialModelSummaryDTO  `json:"official_model"`
	Capabilities    *EffectiveCapabilitiesDTO `json:"capabilities"`
}

type AgentOptionsResponse struct {
	List []AgentOption `json:"list"`
}

type ConnectionTester interface {
	TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error)
}

type HTTPService interface {
	PageInit(ctx context.Context) (*InitResponse, *apperror.Error)
	ProviderModels(ctx context.Context, providerID uint64) (*ProviderModelsResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	Test(ctx context.Context, id uint64) (*infraai.TestConnectionResult, *apperror.Error)
	Delete(ctx context.Context, id uint64) *apperror.Error
	Options(ctx context.Context, query OptionQuery) (*AgentOptionsResponse, *apperror.Error)
}

type ProviderModelsResponse struct {
	List []ProviderModelDTO `json:"list"`
}

type ProviderModelDTO struct {
	ID                     uint64                      `json:"id"`
	ProviderID             uint64                      `json:"provider_id"`
	ModelID                string                      `json:"model_id"`
	DisplayName            string                      `json:"display_name"`
	OfficialModelID        string                      `json:"official_model_id"`
	OfficialCatalogVersion string                      `json:"official_catalog_version"`
	MappingStatus          officialmodel.MappingStatus `json:"mapping_status"`
	MappedAt               string                      `json:"mapped_at"`
	Status                 int                         `json:"status"`
	StatusName             string                      `json:"status_name"`
	CreatedAt              string                      `json:"created_at"`
	UpdatedAt              string                      `json:"updated_at"`
}
