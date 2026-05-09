package aiengine

import (
	"context"
	"encoding/json"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/ai/provider"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	EngineTypeArr   []dict.Option[string] `json:"engine_type_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	HealthStatusArr []dict.Option[string] `json:"health_status_arr"`
	ModelSyncArr    []dict.Option[string] `json:"model_sync_arr"`
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
	List []ConnectionDTO `json:"list"`
	Page Page            `json:"page"`
}

type ConnectionDTO struct {
	ID                  uint64             `json:"id"`
	Name                string             `json:"name"`
	EngineType          string             `json:"engine_type"`
	EngineTypeName      string             `json:"engine_type_name"`
	Driver              string             `json:"driver"`
	DriverName          string             `json:"driver_name"`
	BaseURL             string             `json:"base_url"`
	BaseURLEffective    string             `json:"base_url_effective"`
	APIKeyMasked        string             `json:"api_key_masked"`
	WorkspaceID         string             `json:"workspace_id"`
	HealthStatus        string             `json:"health_status"`
	LastCheckedAt       string             `json:"last_checked_at"`
	LastCheckError      string             `json:"last_check_error"`
	LastModelSyncAt     string             `json:"last_model_sync_at"`
	LastModelSyncStatus string             `json:"last_model_sync_status"`
	LastModelSyncError  string             `json:"last_model_sync_error"`
	EnabledModelCount   int                `json:"enabled_model_count"`
	DefaultModelID      string             `json:"default_model_id"`
	Models              []ProviderModelDTO `json:"models"`
	Status              int                `json:"status"`
	StatusName          string             `json:"status_name"`
	CreatedAt           string             `json:"created_at"`
	UpdatedAt           string             `json:"updated_at"`
}

type ProviderModelDTO struct {
	ID          uint64          `json:"id"`
	ProviderID  uint64          `json:"provider_id"`
	ModelID     string          `json:"model_id"`
	DisplayName string          `json:"display_name"`
	IsDefault   int             `json:"is_default"`
	Source      string          `json:"source"`
	Raw         json.RawMessage `json:"raw"`
	Status      int             `json:"status"`
	StatusName  string          `json:"status_name"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type ModelOptionDTO struct {
	ModelID     string          `json:"model_id"`
	DisplayName string          `json:"display_name"`
	OwnedBy     string          `json:"owned_by"`
	Raw         json.RawMessage `json:"raw"`
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
	Driver            string
	BaseURL           string
	APIKey            string
	WorkspaceID       string
	ModelIDs          []string
	DefaultModelID    string
	ModelDisplayNames map[string]string
	Status            int
}

type UpdateInput = CreateInput

type ModelOptionsInput struct {
	EngineType string
	Driver     string
	BaseURL    string
	APIKey     string
}

type UpdateModelsInput struct {
	ModelIDs          []string
	DefaultModelID    string
	ModelDisplayNames map[string]string
	Statuses          map[string]int
}

type ConnectionTester interface {
	TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error)
}

type ModelDriver interface {
	ListModels(ctx context.Context, cfg provider.Config) ([]provider.Model, error)
	TestConnection(ctx context.Context, cfg provider.Config) (*provider.TestResult, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	TestConnection(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error)
	PreviewModels(ctx context.Context, input ModelOptionsInput) (*ModelOptionsResponse, *apperror.Error)
	SyncModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error)
	ListProviderModels(ctx context.Context, id uint64) (*ProviderModelsResponse, *apperror.Error)
	UpdateProviderModels(ctx context.Context, id uint64, input UpdateModelsInput) *apperror.Error
	Delete(ctx context.Context, id uint64) *apperror.Error
}
