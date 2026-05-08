package aiengine

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	platformai "admin_back_go/internal/platform/ai"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	EngineTypeArr   []dict.Option[string] `json:"engine_type_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	HealthStatusArr []dict.Option[string] `json:"health_status_arr"`
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
	ID             uint64 `json:"id"`
	Name           string `json:"name"`
	EngineType     string `json:"engine_type"`
	EngineTypeName string `json:"engine_type_name"`
	BaseURL        string `json:"base_url"`
	APIKeyMasked   string `json:"api_key_masked"`
	WorkspaceID    string `json:"workspace_id"`
	HealthStatus   string `json:"health_status"`
	LastCheckedAt  string `json:"last_checked_at"`
	Status         int    `json:"status"`
	StatusName     string `json:"status_name"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type CreateInput struct {
	Name        string
	EngineType  string
	BaseURL     string
	APIKey      string
	WorkspaceID string
	Status      int
}

type UpdateInput = CreateInput

type ConnectionTester interface {
	TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	TestConnection(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error)
	Delete(ctx context.Context, id uint64) *apperror.Error
}
