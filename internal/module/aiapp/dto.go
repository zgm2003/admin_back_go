package aiapp

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
	AppTypeArr      []dict.Option[string] `json:"app_type_arr"`
	ResponseModeArr []dict.Option[string] `json:"response_mode_arr"`
	BindingTypeArr  []dict.Option[string] `json:"binding_type_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	ProviderOptions []EngineOption        `json:"provider_options"`
}

type EngineOption struct {
	Label      string `json:"label"`
	Value      uint64 `json:"value"`
	EngineType string `json:"engine_type"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Code        string
	AppType     string
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
	List []AppDTO `json:"list"`
	Page Page     `json:"page"`
}

type DetailResponse struct {
	AppDTO
}

type AppDTO struct {
	ID                      uint64         `json:"id"`
	ProviderID              uint64         `json:"provider_id"`
	ProviderName            string         `json:"provider_name"`
	EngineType              string         `json:"engine_type"`
	Name                    string         `json:"name"`
	Code                    string         `json:"code"`
	AppType                 string         `json:"app_type"`
	AppTypeName             string         `json:"app_type_name"`
	EngineAppID             string         `json:"engine_app_id"`
	EngineAppAPIKeyMasked   string         `json:"engine_app_api_key_masked"`
	DefaultResponseMode     string         `json:"default_response_mode"`
	DefaultResponseModeName string         `json:"default_response_mode_name"`
	RuntimeConfig           map[string]any `json:"runtime_config"`
	Status                  int            `json:"status"`
	StatusName              string         `json:"status_name"`
	CreatedAt               string         `json:"created_at"`
	UpdatedAt               string         `json:"updated_at"`
}

type CreateInput struct {
	ProviderID          uint64
	Name                string
	Code                string
	AppType             string
	EngineAppID         string
	EngineAppAPIKey     string
	DefaultResponseMode string
	RuntimeConfig       map[string]any
	Status              int
}

type UpdateInput = CreateInput

type BindingInput struct {
	BindType string
	BindKey  string
	Sort     int
	Status   int
}

type BindingListResponse struct {
	List []BindingDTO `json:"list"`
}

type BindingDTO struct {
	ID           uint64 `json:"id"`
	AppID        uint64 `json:"app_id"`
	BindType     string `json:"bind_type"`
	BindTypeName string `json:"bind_type_name"`
	BindKey      string `json:"bind_key"`
	Sort         int    `json:"sort"`
	Status       int    `json:"status"`
	StatusName   string `json:"status_name"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type OptionQuery struct {
	UserID   int64
	RoleID   int64
	Platform string
}

type AppOption struct {
	Label string `json:"label"`
	Value uint64 `json:"value"`
	Code  string `json:"code"`
}

type AppOptionsResponse struct {
	List []AppOption `json:"list"`
}

type ConnectionTester interface {
	TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	Test(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error)
	Delete(ctx context.Context, id uint64) *apperror.Error
	Bindings(ctx context.Context, appID uint64) (*BindingListResponse, *apperror.Error)
	CreateBinding(ctx context.Context, appID uint64, input BindingInput) (uint64, *apperror.Error)
	DeleteBinding(ctx context.Context, id uint64) *apperror.Error
	Options(ctx context.Context, query OptionQuery) (*AppOptionsResponse, *apperror.Error)
}
