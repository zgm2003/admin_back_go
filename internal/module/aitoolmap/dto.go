package aitoolmap

import (
	"context"
	"encoding/json"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	ToolTypeArr             []dict.Option[string] `json:"tool_type_arr"`
	RiskLevelArr            []dict.Option[string] `json:"risk_level_arr"`
	CommonStatusArr         []dict.Option[int]    `json:"common_status_arr"`
	EngineConnectionOptions []EngineOption        `json:"engine_connection_options"`
}

type EngineOption struct {
	Label      string `json:"label"`
	Value      uint64 `json:"value"`
	EngineType string `json:"engine_type"`
}

type ListQuery struct {
	CurrentPage        int
	PageSize           int
	Name               string
	Code               string
	ToolType           string
	RiskLevel          string
	EngineConnectionID uint64
	AppID              *uint64
	Status             *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ToolMapDTO `json:"list"`
	Page Page         `json:"page"`
}

type ToolMapDTO struct {
	ID                   uint64          `json:"id"`
	EngineConnectionID   uint64          `json:"engine_connection_id"`
	EngineConnectionName string          `json:"engine_connection_name"`
	EngineType           string          `json:"engine_type"`
	AppID                *uint64         `json:"app_id"`
	Name                 string          `json:"name"`
	Code                 string          `json:"code"`
	ToolType             string          `json:"tool_type"`
	ToolTypeName         string          `json:"tool_type_name"`
	EngineToolID         string          `json:"engine_tool_id"`
	PermissionCode       string          `json:"permission_code"`
	RiskLevel            string          `json:"risk_level"`
	RiskLevelName        string          `json:"risk_level_name"`
	ConfigJSON           json.RawMessage `json:"config_json"`
	Status               int             `json:"status"`
	StatusName           string          `json:"status_name"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
}

type MutationInput struct {
	EngineConnectionID uint64
	AppID              *uint64
	Name               string
	Code               string
	ToolType           string
	EngineToolID       string
	PermissionCode     string
	RiskLevel          string
	ConfigJSON         json.RawMessage
	Status             int
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input MutationInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input MutationInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	Delete(ctx context.Context, id uint64) *apperror.Error
}
