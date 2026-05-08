package aitool

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type JSONObject map[string]any

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	AIExecutorTypeArr []dict.Option[int] `json:"ai_executor_type_arr"`
	CommonStatusArr   []dict.Option[int] `json:"common_status_arr"`
}

type ListQuery struct {
	CurrentPage  int
	PageSize     int
	Name         string
	Status       *int
	ExecutorType *int
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
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Code           string     `json:"code"`
	Description    string     `json:"description"`
	SchemaJSON     JSONObject `json:"schema_json"`
	ExecutorType   int        `json:"executor_type"`
	ExecutorName   string     `json:"executor_name"`
	ExecutorConfig JSONObject `json:"executor_config"`
	Status         int        `json:"status"`
	StatusName     string     `json:"status_name"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

type CreateInput struct {
	Name           string
	Code           string
	Description    string
	SchemaJSON     JSONObject
	ExecutorType   int
	ExecutorConfig JSONObject
	Status         int
}

type UpdateInput struct {
	Name           string
	Code           string
	Description    string
	SchemaJSON     JSONObject
	ExecutorType   int
	ExecutorConfig JSONObject
	Status         int
}

type ToolOption struct {
	Value int64  `json:"value"`
	Label string `json:"label"`
	Code  string `json:"code"`
}

type AgentToolsResponse struct {
	BoundToolIDs []int64      `json:"bound_tool_ids"`
	AllTools     []ToolOption `json:"all_tools"`
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
	AgentOptions(ctx context.Context, agentID int64) (*AgentToolsResponse, *apperror.Error)
	SyncAgentBindings(ctx context.Context, agentID int64, toolIDs []int64) *apperror.Error
}
