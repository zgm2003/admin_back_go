package aitool

import (
	"context"
	"encoding/json"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type JSONObject = map[string]any

const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"

	ToolCallRunning = "running"
	ToolCallSuccess = "success"
	ToolCallFailed  = "failed"
	ToolCallTimeout = "timeout"
)

var RiskLevelLabels = map[string]string{
	RiskLow:    "低",
	RiskMedium: "中",
	RiskHigh:   "高",
}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	RiskLevelArr    []dict.Option[string] `json:"risk_level_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Code        string
	RiskLevel   string
	Status      *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ToolDTO `json:"list"`
	Page Page      `json:"page"`
}

type ToolDTO struct {
	ID               uint64          `json:"id"`
	Name             string          `json:"name"`
	Code             string          `json:"code"`
	Description      string          `json:"description"`
	ParametersJSON   json.RawMessage `json:"parameters_json"`
	ResultSchemaJSON json.RawMessage `json:"result_schema_json"`
	RiskLevel        string          `json:"risk_level"`
	RiskLevelName    string          `json:"risk_level_name"`
	TimeoutMS        uint            `json:"timeout_ms"`
	Status           int             `json:"status"`
	StatusName       string          `json:"status_name"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

type MutationInput struct {
	Name             string
	Code             string
	Description      string
	ParametersJSON   json.RawMessage
	ResultSchemaJSON json.RawMessage
	RiskLevel        string
	TimeoutMS        uint
	Status           int
}

type AgentToolsResponse struct {
	AgentID       uint64   `json:"agent_id"`
	ToolIDs       []uint64 `json:"tool_ids"`
	ActiveToolIDs []uint64 `json:"active_tool_ids"`
}

type UpdateAgentToolsInput struct {
	ToolIDs []uint64
}

type RuntimeToolRow struct {
	ToolID           uint64
	Name             string
	Code             string
	Description      string
	ParametersJSON   string
	ResultSchemaJSON string
	RiskLevel        string
	TimeoutMS        uint
	ToolStatus       int
	BindingStatus    int
}

type RuntimeTool struct {
	ID               uint64
	Name             string
	Code             string
	Description      string
	ParametersJSON   map[string]any
	ResultSchemaJSON map[string]any
	RiskLevel        string
	TimeoutMS        uint
}

type StartToolCallInput struct {
	RunID         uint64
	ToolID        uint64
	ToolCode      string
	ToolName      string
	CallID        string
	ArgumentsJSON json.RawMessage
	StartedAt     time.Time
}

type FinishToolCallInput struct {
	ID           uint64
	Status       string
	ResultJSON   *json.RawMessage
	ErrorMessage string
	DurationMS   uint
	FinishedAt   time.Time
}

type ExecuteInput struct {
	RunID     uint64
	Tool      RuntimeTool
	CallID    string
	Arguments json.RawMessage
}

type ExecuteResult struct {
	CallID string
	Name   string
	Output json.RawMessage
}

type UserCount struct {
	TotalUsers    int64 `json:"total_users"`
	EnabledUsers  int64 `json:"enabled_users"`
	DisabledUsers int64 `json:"disabled_users"`
}

type Executor interface {
	Execute(ctx context.Context, arguments json.RawMessage) (map[string]any, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input MutationInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input MutationInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	Delete(ctx context.Context, id uint64) *apperror.Error
	AgentTools(ctx context.Context, agentID uint64) (*AgentToolsResponse, *apperror.Error)
	UpdateAgentTools(ctx context.Context, agentID uint64, input UpdateAgentToolsInput) *apperror.Error
}

type RuntimeService interface {
	ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeTool, *apperror.Error)
	Execute(ctx context.Context, input ExecuteInput) (*ExecuteResult, *apperror.Error)
}
