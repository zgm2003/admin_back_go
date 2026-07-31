package aiprovider

import (
	"context"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/provider"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
)

const (
	FileInputModeDisabled        = "disabled"
	FileInputModeChatCompletions = "chat_completions"
)

var FileInputModes = []string{FileInputModeDisabled, FileInputModeChatCompletions}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	EngineTypeArr    []dict.Option[string] `json:"engine_type_arr"`
	FileInputModeArr []FileInputModeOption `json:"file_input_mode_arr"`
	CommonStatusArr  []dict.Option[int]    `json:"common_status_arr"`
	HealthStatusArr  []dict.Option[string] `json:"health_status_arr"`
	ModelSyncArr     []dict.Option[string] `json:"model_sync_arr"`
}

type FileInputModeOption struct {
	Label string `json:"label"`
	Value string `json:"value" validate:"oneof=disabled chat_completions"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	EngineType  string
	Status      *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ProviderDTO `json:"list"`
	Page Page          `json:"page"`
}

type ProviderDTO struct {
	ID                  uint64             `json:"id"`
	Name                string             `json:"name"`
	EngineType          string             `json:"engine_type"`
	EngineTypeName      string             `json:"engine_type_name"`
	BaseURL             string             `json:"base_url"`
	BaseURLEffective    string             `json:"base_url_effective"`
	FileInputMode       string             `json:"file_input_mode" validate:"oneof=disabled chat_completions"`
	APIKeyMasked        string             `json:"api_key_masked"`
	HealthStatus        string             `json:"health_status"`
	LastCheckedAt       string             `json:"last_checked_at"`
	LastCheckError      string             `json:"last_check_error"`
	LastModelSyncAt     string             `json:"last_model_sync_at"`
	LastModelSyncStatus string             `json:"last_model_sync_status"`
	LastModelSyncError  string             `json:"last_model_sync_error"`
	EnabledModelCount   int                `json:"enabled_model_count"`
	Models              []ProviderModelDTO `json:"models"`
	Status              int                `json:"status"`
	StatusName          string             `json:"status_name"`
	CreatedAt           string             `json:"created_at"`
	UpdatedAt           string             `json:"updated_at"`
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

type ModelOptionDTO struct {
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
	OwnedBy     string `json:"owned_by"`
}

type ModelOptionsResponse struct {
	List []ModelOptionDTO `json:"list"`
}

type ProviderModelsResponse struct {
	List []ProviderModelDTO `json:"list"`
}

type CreateInput struct {
	Name              string
	EngineType        string
	BaseURL           string
	APIKey            string
	FileInputMode     string
	ModelIDs          []string
	ModelDisplayNames map[string]string
	Status            int
}

type UpdateInput = CreateInput

type ModelOptionsInput struct {
	EngineType string
	BaseURL    string
	APIKey     string
}

type UpdateModelsInput struct {
	ModelIDs          []string
	ModelDisplayNames map[string]string
	Statuses          map[string]int
}

type ProviderTester interface {
	TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error)
}

type ModelDriver interface {
	ListModels(ctx context.Context, cfg provider.Config) ([]provider.Model, error)
	TestConnection(ctx context.Context, cfg provider.Config) (*provider.TestResult, error)
}

type HTTPService interface {
	PageInit(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	TestConnection(ctx context.Context, id uint64) (*infraai.TestConnectionResult, *apperror.Error)
	PreviewModels(ctx context.Context, input ModelOptionsInput) (*ModelOptionsResponse, *apperror.Error)
	PreviewStoredModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error)
	SyncModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error)
	ListProviderModels(ctx context.Context, id uint64) (*ProviderModelsResponse, *apperror.Error)
	UpdateProviderModels(ctx context.Context, id uint64, input UpdateModelsInput) *apperror.Error
	Delete(ctx context.Context, id uint64) *apperror.Error
}
