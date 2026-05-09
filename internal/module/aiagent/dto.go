package aiagent

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
	SceneArr        []dict.Option[string] `json:"scene_arr"`
	BindingTypeArr  []dict.Option[string] `json:"binding_type_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	ProviderOptions []EngineOption        `json:"provider_options"`
	ModelOptions    []ModelOption         `json:"provider_model_options"`
}

type EngineOption struct {
	Label      string `json:"label"`
	Value      uint64 `json:"value"`
	EngineType string `json:"engine_type"`
}

type ModelOption struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	ProviderID  uint64 `json:"provider_id"`
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
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
	ID               uint64   `json:"id"`
	ProviderID       uint64   `json:"provider_id"`
	ProviderName     string   `json:"provider_name"`
	EngineType       string   `json:"engine_type"`
	Name             string   `json:"name"`
	ModelID          string   `json:"model_id"`
	ModelDisplayName string   `json:"model_display_name"`
	Scenes           []string `json:"scenes"`
	SceneNames       []string `json:"scene_names"`
	SystemPrompt     string   `json:"system_prompt"`
	Avatar           string   `json:"avatar"`
	Status           int      `json:"status"`
	StatusName       string   `json:"status_name"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type CreateInput struct {
	ProviderID   uint64
	Name         string
	ModelID      string
	Scenes       []string
	SystemPrompt string
	Avatar       string
	Status       int
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
	AgentID      uint64 `json:"agent_id"`
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

type AgentOption struct {
	Label string `json:"label"`
	Value uint64 `json:"value"`
}

type AgentOptionsResponse struct {
	List []AgentOption `json:"list"`
}

type ConnectionTester interface {
	TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	ProviderModels(ctx context.Context, providerID uint64) (*ProviderModelsResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	Test(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error)
	Delete(ctx context.Context, id uint64) *apperror.Error
	Bindings(ctx context.Context, agentID uint64) (*BindingListResponse, *apperror.Error)
	CreateBinding(ctx context.Context, agentID uint64, input BindingInput) (uint64, *apperror.Error)
	DeleteBinding(ctx context.Context, id uint64) *apperror.Error
	Options(ctx context.Context, query OptionQuery) (*AgentOptionsResponse, *apperror.Error)
}

type ProviderModelsResponse struct {
	List []ProviderModelDTO `json:"list"`
}

type ProviderModelDTO struct {
	ID          uint64 `json:"id"`
	ProviderID  uint64 `json:"provider_id"`
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
	Status      int    `json:"status"`
	StatusName  string `json:"status_name"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
