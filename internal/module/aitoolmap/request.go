package aitoolmap

import "encoding/json"

type listRequest struct {
	CurrentPage int     `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int     `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string  `form:"name" binding:"max=128"`
	Code        string  `form:"code" binding:"max=128"`
	ToolType    string  `form:"tool_type" binding:"omitempty,oneof=dify_tool workflow_node admin_action_gateway http_reference"`
	RiskLevel   string  `form:"risk_level" binding:"omitempty,oneof=low medium high"`
	ProviderID  uint64  `form:"provider_id" binding:"omitempty,gt=0"`
	AppID       *uint64 `form:"app_id" binding:"omitempty,gt=0"`
	Status      *int    `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	ProviderID     uint64          `json:"provider_id" binding:"required,gt=0"`
	AppID          *uint64         `json:"app_id" binding:"omitempty,gt=0"`
	Name           string          `json:"name" binding:"required,max=128"`
	Code           string          `json:"code" binding:"required,max=128"`
	ToolType       string          `json:"tool_type" binding:"required,oneof=dify_tool workflow_node admin_action_gateway http_reference"`
	EngineToolID   string          `json:"engine_tool_id" binding:"omitempty,max=128"`
	PermissionCode string          `json:"permission_code" binding:"omitempty,max=128"`
	RiskLevel      string          `json:"risk_level" binding:"required,oneof=low medium high"`
	ConfigJSON     json.RawMessage `json:"config_json"`
	Status         int             `json:"status" binding:"required,oneof=1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
