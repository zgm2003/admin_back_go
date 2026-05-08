package aitool

type listRequest struct {
	CurrentPage  int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize     int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name         string `form:"name" binding:"max=50"`
	Status       *int   `form:"status" binding:"omitempty,common_status"`
	ExecutorType *int   `form:"executor_type"`
}

type mutationRequest struct {
	Name           string     `json:"name" binding:"required,max=50"`
	Code           string     `json:"code" binding:"required,max=60"`
	Description    string     `json:"description" binding:"max=255"`
	SchemaJSON     JSONObject `json:"schema_json"`
	ExecutorType   int        `json:"executor_type" binding:"required"`
	ExecutorConfig JSONObject `json:"executor_config"`
	Status         int        `json:"status" binding:"omitempty,common_status"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type bindingsRequest struct {
	ToolIDs []int64 `json:"tool_ids" binding:"required"`
}

type agentOptionsRequest struct {
	AgentID int64 `form:"agent_id" binding:"omitempty,min=1"`
}
