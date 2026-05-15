package aiimage

import (
	"context"
	"encoding/json"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type PageInitResponse struct {
	Dict         PageInitDict  `json:"dict"`
	AgentOptions []AgentOption `json:"agent_options"`
}

type PageInitDict struct {
	SizeArr         []dict.Option[string] `json:"size_arr"`
	QualityArr      []dict.Option[string] `json:"quality_arr"`
	OutputFormatArr []dict.Option[string] `json:"output_format_arr"`
	ModerationArr   []dict.Option[string] `json:"moderation_arr"`
	StatusArr       []dict.Option[string] `json:"status_arr"`
	FavoriteArr     []dict.Option[int]    `json:"favorite_arr"`
}

type AgentOption struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Status      string
	IsFavorite  int
}

type ListResponse struct {
	List []TaskDTO `json:"list"`
	Page Page      `json:"page"`
}

type DetailResponse struct {
	Task    TaskDTO    `json:"task"`
	Inputs  []AssetDTO `json:"inputs"`
	Mask    *AssetDTO  `json:"mask"`
	Outputs []AssetDTO `json:"outputs"`
}

type TaskDTO struct {
	ID                       uint64          `json:"id"`
	AgentID                  uint64          `json:"agent_id"`
	AgentNameSnapshot        string          `json:"agent_name_snapshot"`
	ProviderIDSnapshot       uint64          `json:"provider_id_snapshot"`
	ProviderNameSnapshot     string          `json:"provider_name_snapshot"`
	ModelIDSnapshot          string          `json:"model_id_snapshot"`
	ModelDisplayNameSnapshot string          `json:"model_display_name_snapshot"`
	Prompt                   string          `json:"prompt"`
	Size                     string          `json:"size"`
	Quality                  string          `json:"quality"`
	OutputFormat             string          `json:"output_format"`
	OutputCompression        *int            `json:"output_compression"`
	Moderation               string          `json:"moderation"`
	N                        int             `json:"n"`
	Status                   string          `json:"status"`
	StatusName               string          `json:"status_name"`
	ErrorMessage             string          `json:"error_message"`
	ActualParamsJSON         json.RawMessage `json:"actual_params_json"`
	IsFavorite               int             `json:"is_favorite"`
	FinishedAt               string          `json:"finished_at"`
	ElapsedMS                int             `json:"elapsed_ms"`
	CreatedAt                string          `json:"created_at"`
	UpdatedAt                string          `json:"updated_at"`
}

type AssetDTO struct {
	ID               uint64          `json:"id"`
	StorageProvider  string          `json:"storage_provider"`
	StorageKey       string          `json:"storage_key"`
	StorageURL       string          `json:"storage_url"`
	MimeType         string          `json:"mime_type"`
	Width            int             `json:"width"`
	Height           int             `json:"height"`
	SizeBytes        int64           `json:"size_bytes"`
	SourceType       string          `json:"source_type"`
	Role             string          `json:"role,omitempty"`
	SortOrder        int             `json:"sort_order,omitempty"`
	RelatedAssetID   *uint64         `json:"related_asset_id,omitempty"`
	RevisedPrompt    string          `json:"revised_prompt,omitempty"`
	ActualParamsJSON json.RawMessage `json:"actual_params_json,omitempty"`
	CreatedAt        string          `json:"created_at"`
}

type RegisterAssetInput struct {
	UserID          uint64
	StorageProvider string
	StorageKey      string
	StorageURL      string
	MimeType        string
	Width           int
	Height          int
	SizeBytes       int64
	SourceType      string
}

type CreateInput struct {
	UserID            uint64
	AgentID           uint64
	Prompt            string
	Size              string
	Quality           string
	OutputFormat      string
	OutputCompression *int
	Moderation        string
	N                 int
	InputAssetIDs     []uint64
	MaskAssetID       uint64
	MaskTargetAssetID uint64
}

type FavoriteInput struct {
	UserID     uint64
	TaskID     uint64
	IsFavorite int
}

type CreateTaskResponse struct {
	Task TaskDTO `json:"task"`
}

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	List(ctx context.Context, userID uint64, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, userID uint64, taskID uint64) (*DetailResponse, *apperror.Error)
	RegisterAsset(ctx context.Context, input RegisterAssetInput) (*AssetDTO, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (*CreateTaskResponse, *apperror.Error)
	Favorite(ctx context.Context, input FavoriteInput) (*TaskDTO, *apperror.Error)
	Delete(ctx context.Context, userID uint64, taskID uint64) *apperror.Error
}

type JobService interface {
	ExecuteGenerate(ctx context.Context, input GenerateInput) (*GenerateResult, error)
}
