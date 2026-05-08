package aiagent

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=50"`
	ModelID     *int64 `form:"model_id" binding:"omitempty,min=1"`
	Mode        string `form:"mode" binding:"omitempty,max=20"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name             string       `json:"name" binding:"required,max=50"`
	ModelID          int64        `json:"model_id" binding:"required,min=1"`
	Avatar           *string      `json:"avatar" binding:"omitempty,max=255"`
	SystemPrompt     *string      `json:"system_prompt"`
	Mode             string       `json:"mode" binding:"omitempty,max=20"`
	Scene            *string      `json:"scene" binding:"omitempty,max=60"`
	Capabilities     Capabilities `json:"capabilities"`
	SceneCodes       []string     `json:"scene_codes"`
	RuntimeConfig    JSONObject   `json:"runtime_config"`
	Policy           JSONObject   `json:"policy"`
	Status           int          `json:"status" binding:"omitempty,common_status"`
	ToolIDs          []int64      `json:"tool_ids"`
	KnowledgeBaseIDs []int64      `json:"knowledge_base_ids"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
