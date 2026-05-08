package aiagent

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type JSONObject map[string]any

type Capabilities struct {
	Chat     bool `json:"chat,omitempty"`
	Tools    bool `json:"tools,omitempty"`
	RAG      bool `json:"rag,omitempty"`
	Workflow bool `json:"workflow,omitempty"`
}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	AIModeArr         []dict.Option[string] `json:"ai_mode_arr"`
	AICapabilityArr   []dict.Option[string] `json:"ai_capability_arr"`
	AISceneArr        []dict.Option[string] `json:"ai_scene_arr"`
	CommonStatusArr   []dict.Option[int]    `json:"common_status_arr"`
	ModelList         []dict.Option[int]    `json:"model_list"`
	KnowledgeBaseList []dict.Option[int]    `json:"knowledge_base_list"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	ModelID     *int64
	Mode        string
	Status      *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID                 int64        `json:"id"`
	Name               string       `json:"name"`
	ModelID            int64        `json:"model_id"`
	ModelName          string       `json:"model_name"`
	ModelDeleted       bool         `json:"model_deleted"`
	Driver             string       `json:"driver"`
	DriverName         string       `json:"driver_name"`
	ModelCode          string       `json:"model_code"`
	Avatar             *string      `json:"avatar"`
	SystemPrompt       *string      `json:"system_prompt"`
	Mode               string       `json:"mode"`
	ModeName           string       `json:"mode_name"`
	Scene              *string      `json:"scene"`
	SceneName          string       `json:"scene_name"`
	SceneCodes         []string     `json:"scene_codes"`
	SceneNames         []string     `json:"scene_names"`
	Capabilities       Capabilities `json:"capabilities"`
	RuntimeConfig      JSONObject   `json:"runtime_config"`
	Policy             JSONObject   `json:"policy"`
	KnowledgeBaseIDs   []int64      `json:"knowledge_base_ids"`
	KnowledgeBaseNames []string     `json:"knowledge_base_names"`
	Status             int          `json:"status"`
	StatusName         string       `json:"status_name"`
	CreatedAt          string       `json:"created_at"`
	UpdatedAt          string       `json:"updated_at"`
}

type MutationInput struct {
	Name             string
	ModelID          int64
	Avatar           *string
	SystemPrompt     *string
	Mode             string
	Scene            *string
	Capabilities     Capabilities
	SceneCodes       []string
	RuntimeConfig    JSONObject
	Policy           JSONObject
	Status           int
	ToolIDs          []int64
	KnowledgeBaseIDs []int64
}

type ModelOptionRow struct {
	ID     int64
	Name   string
	Driver string
}
type OptionRow struct {
	ID   int64
	Name string
}
type ListRow struct {
	Agent        Agent
	ModelName    string
	Driver       string
	ModelCode    string
	ModelDeleted bool
}
type BindingData struct {
	SceneCodes         map[int64][]string
	ToolIDs            map[int64][]int64
	KnowledgeBaseIDs   map[int64][]int64
	KnowledgeBaseNames map[int64][]string
}

type Repository interface {
	InitModels(ctx context.Context) ([]ModelOptionRow, error)
	InitKnowledgeBases(ctx context.Context) ([]OptionRow, error)
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Get(ctx context.Context, id int64) (*Agent, error)
	ActiveModelExists(ctx context.Context, id int64) (bool, error)
	ActiveToolIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error)
	ActiveKnowledgeBaseIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error)
	BindingData(ctx context.Context, agentIDs []int64) (BindingData, error)
	WithTx(ctx context.Context, fn func(Repository) error) error
	CreateAgent(ctx context.Context, row Agent) (int64, error)
	UpdateAgent(ctx context.Context, id int64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id int64, status int) error
	SoftDeleteAgentAndBindings(ctx context.Context, id int64) error
	SyncToolBindings(ctx context.Context, agentID int64, toolIDs []int64) error
	SyncKnowledgeBindings(ctx context.Context, agentID int64, knowledgeIDs []int64) error
	SyncSceneBindings(ctx context.Context, agentID int64, sceneCodes []string) error
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input MutationInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input MutationInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
}
