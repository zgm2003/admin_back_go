package canvas

import (
	"context"

	infraai "admin_back_go/internal/infra/ai"
	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/apperror"
)

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type PromptListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	Category    string
	Status      int
	IsDel       int
}

type AssetListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	Type        string
	Status      int
	IsDel       int
}

type PromptInput struct {
	Slug      string
	Category  string
	Title     string
	CoverURL  string
	Prompt    string
	Preview   string
	TagsJSON  string
	SourceURL string
	Status    int
}

type AssetInput struct {
	Slug        string
	Type        string
	Category    string
	Title       string
	CoverURL    string
	Description string
	Content     string
	URL         string
	TagsJSON    string
	Status      int
}

type PromptListResponse struct {
	List []PromptItem `json:"list"`
	Page Page         `json:"page"`
}

type AssetListResponse struct {
	List []AssetItem `json:"list"`
	Page Page        `json:"page"`
}

type PromptItem struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Category  string `json:"category"`
	Title     string `json:"title"`
	CoverURL  string `json:"cover_url"`
	Prompt    string `json:"prompt"`
	Preview   string `json:"preview"`
	TagsJSON  string `json:"tags_json"`
	SourceURL string `json:"source_url"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type AssetItem struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	TagsJSON    string `json:"tags_json"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type SettingsInput struct {
	UserID int64
}

type SettingsResponse struct {
	AllowRegister bool              `json:"allow_register"`
	Scenes        []string          `json:"scenes"`
	Agents        CanvasAgentGroups `json:"agents"`
}

type CanvasAgentGroups struct {
	Text  []CanvasAgentOption `json:"text"`
	Image []CanvasAgentOption `json:"image"`
	Video []CanvasAgentOption `json:"video"`
}

type CanvasAgentOption struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	ModelID          string `json:"model_id"`
	ModelDisplayName string `json:"model_display_name"`
	Scene            string `json:"scene"`
}

type ChatCompletionInput struct {
	UserID  int64
	AgentID int64
	ModelID string
	Message string
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Content string `json:"content"`
}

type ImageGenerationInput struct {
	UserID            int64
	AgentID           int64
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

type ImageGenerationResponse struct {
	TaskID uint64 `json:"task_id"`
	Status string `json:"status"`
}

type VideoGenerationInput struct {
	UserID          int64
	AgentID         int64
	ModelID         string
	Prompt          string
	DurationSeconds int
	Size            string
	ResolutionName  string
}

type VideoGenerationResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type VideoStatusResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type VideoRuntime interface {
	Create(ctx context.Context, input VideoCreateInput) (*VideoCreateResult, *apperror.Error)
	Task(ctx context.Context, userID int64, id int64) (*VideoTask, *apperror.Error)
	Status(ctx context.Context, input VideoStatusInput) (*VideoProviderStatus, *apperror.Error)
	Content(ctx context.Context, input VideoContentInput) ([]byte, string, *apperror.Error)
}

type VideoAgentRuntime struct {
	AgentID          int64
	ProviderID       int64
	ModelID          string
	ModelDisplayName string
	SystemPrompt     string
	ScenesJSON       string
	EngineType       string
	EngineBaseURL    string
	EngineAPIKeyEnc  string
	AgentStatus      int
	EngineStatus     int
}

type VideoRepository interface {
	AgentForVideoRuntime(ctx context.Context, agentID int64) (*VideoAgentRuntime, error)
	CreateVideoTask(ctx context.Context, task VideoTask) (int64, error)
	UpdateVideoTask(ctx context.Context, userID int64, id int64, fields map[string]any) error
	GetVideoTask(ctx context.Context, userID int64, id int64) (*VideoTask, error)
}

type VideoEngineFactory interface {
	NewVideoEngine(ctx context.Context, input VideoEngineConfig) (infraai.VideoEngine, error)
}

type VideoEngineConfig struct {
	EngineType infraai.EngineType
	BaseURL    string
	APIKey     string
}

type VideoCreateInput struct {
	UserID          int64
	AgentID         int64
	ModelID         string
	Prompt          string
	DurationSeconds int
	Size            string
	ResolutionName  string
}

type VideoCreateResult struct {
	ID             int64
	ProviderID     int64
	ModelID        string
	ProviderTaskID string
	Status         string
}

type VideoStatusInput struct {
	UserID int64
	Task   *VideoTask
}

type VideoContentInput struct {
	UserID int64
	Task   *VideoTask
}

type VideoProviderStatus struct {
	Status       string
	ErrorMessage string
}

type ImageRuntime interface {
	Create(ctx context.Context, input aiimagemodule.CreateInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error)
}

type TextRuntime interface {
	Generate(ctx context.Context, input TextGenerationInput) (*TextGenerationResponse, *apperror.Error)
}

type TextGenerationInput struct {
	UserID  int64
	AgentID int64
	ModelID string
	Message string
}

type TextGenerationResponse struct {
	Content string
}

type TextAgentRuntime struct {
	AgentID          int64
	ProviderID       int64
	ModelID          string
	ModelDisplayName string
	SystemPrompt     string
	ScenesJSON       string
	EngineType       string
	EngineBaseURL    string
	EngineAPIKeyEnc  string
	AgentStatus      int
	EngineStatus     int
}

type TextRepository interface {
	AgentForTextRuntime(ctx context.Context, agentID int64) (*TextAgentRuntime, error)
}

type TextEngineFactory interface {
	NewEngine(ctx context.Context, input TextEngineConfig) (infraai.Engine, error)
}

type TextEngineConfig struct {
	EngineType infraai.EngineType
	BaseURL    string
	APIKey     string
}

type Secretbox interface {
	Decrypt(cipherText string) (string, error)
}
