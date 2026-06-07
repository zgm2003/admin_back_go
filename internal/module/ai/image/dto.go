package aiimage

import (
	"context"
	"encoding/json"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
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
}

type AgentOption struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Platform    string
	Status      string
}

type ListResponse struct {
	List []TaskDTO `json:"list"`
	Page Page      `json:"page"`
}

type DetailResponse struct {
	Task    TaskDTO   `json:"task"`
	Inputs  []FileDTO `json:"inputs"`
	Mask    *FileDTO  `json:"mask"`
	Outputs []FileDTO `json:"outputs"`
}

type TaskDTO struct {
	ID                       uint64          `json:"id"`
	Platform                 string          `json:"platform"`
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
	FinishedAt               string          `json:"finished_at"`
	ElapsedMS                int             `json:"elapsed_ms"`
	CreatedAt                string          `json:"created_at"`
	UpdatedAt                string          `json:"updated_at"`
}

type FileDTO struct {
	ID              uint64  `json:"id"`
	TaskID          uint64  `json:"task_id"`
	Role            string  `json:"role"`
	SortOrder       int     `json:"sort_order"`
	StorageProvider string  `json:"storage_provider"`
	StorageKey      string  `json:"storage_key"`
	StorageURL      string  `json:"storage_url"`
	MimeType        string  `json:"mime_type"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	SizeBytes       int64   `json:"size_bytes"`
	RelatedFileID   *uint64 `json:"related_file_id,omitempty"`
	RevisedPrompt   string  `json:"revised_prompt,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type ImageFileInput struct {
	StorageProvider string
	StorageKey      string
	StorageURL      string
	MimeType        string
	Width           int
	Height          int
	SizeBytes       int64
}

type MaskFileInput struct {
	ImageFileInput
	RelatedSortOrder int
}

type CreateInput struct {
	UserID            uint64
	AgentID           uint64
	Platform          string
	Prompt            string
	Size              string
	Quality           string
	OutputFormat      string
	OutputCompression *int
	Moderation        string
	N                 int
	InputFiles        []ImageFileInput
	MaskFile          *MaskFileInput
}

type UploadedFileInput struct {
	FileName string
	MimeType string
	Body     []byte
}

type CreateWithUploadedFilesInput struct {
	CreateInput
	Files []UploadedFileInput
}

type CreateTaskResponse struct {
	Task TaskDTO `json:"task"`
}

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	List(ctx context.Context, userID uint64, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, userID uint64, taskID uint64, platform string) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (*CreateTaskResponse, *apperror.Error)
	CreateWithUploadedFiles(ctx context.Context, input CreateWithUploadedFilesInput) (*CreateTaskResponse, *apperror.Error)
	Delete(ctx context.Context, userID uint64, taskID uint64, platform string) *apperror.Error
}

type JobService interface {
	ExecuteGenerate(ctx context.Context, input GenerateInput) (*GenerateResult, error)
}
