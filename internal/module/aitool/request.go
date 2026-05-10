package aitool

import "encoding/json"

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"omitempty,max=128"`
	Code        string `form:"code" binding:"omitempty,max=128"`
	RiskLevel   string `form:"risk_level" binding:"omitempty,oneof=low medium high"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name             string          `json:"name" binding:"required,max=128"`
	Code             string          `json:"code" binding:"required,max=128"`
	Description      string          `json:"description" binding:"omitempty,max=1024"`
	ParametersJSON   json.RawMessage `json:"parameters_json" binding:"required"`
	ResultSchemaJSON json.RawMessage `json:"result_schema_json" binding:"required"`
	RiskLevel        string          `json:"risk_level" binding:"required,oneof=low medium high"`
	TimeoutMS        uint            `json:"timeout_ms" binding:"required,min=100,max=30000"`
	Status           int             `json:"status" binding:"required,oneof=1 2"`
}

type generateDraftRequest struct {
	AgentID     uint64 `json:"agent_id" binding:"required,gt=0"`
	Requirement string `json:"requirement" binding:"required,max=4000"`
	CodeHint    string `json:"code_hint" binding:"omitempty,max=64"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}

type updateAgentToolsRequest struct {
	ToolIDs []uint64 `json:"tool_ids" binding:"omitempty,dive,gt=0"`
}
