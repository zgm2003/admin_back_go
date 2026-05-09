package aiagent

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=128"`
	Code        string `form:"code" binding:"max=128"`
	AgentType   string `form:"agent_type" binding:"omitempty,oneof=chat workflow completion agent"`
	ProviderID  uint64 `form:"provider_id" binding:"omitempty,gt=0"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	ProviderID          uint64         `json:"provider_id" binding:"required,gt=0"`
	Name                string         `json:"name" binding:"required,max=128"`
	Code                string         `json:"code" binding:"required,max=128"`
	AgentType           string         `json:"agent_type" binding:"omitempty,oneof=chat workflow completion agent"`
	ModelID             string         `json:"model_id" binding:"required,max=191"`
	Scenes              []string       `json:"scenes" binding:"required,min=1,dive,required,max=32"`
	SystemPrompt        string         `json:"system_prompt" binding:"omitempty,max=20000"`
	Avatar              string         `json:"avatar" binding:"omitempty,max=512"`
	ExternalAgentID     string         `json:"external_agent_id" binding:"omitempty,max=128"`
	ExternalAgentAPIKey string         `json:"external_agent_api_key" binding:"omitempty,max=4096"`
	DefaultResponseMode string         `json:"default_response_mode" binding:"omitempty,oneof=streaming blocking"`
	RuntimeConfig       map[string]any `json:"runtime_config"`
	Status              int            `json:"status" binding:"required,oneof=1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}

type bindingRequest struct {
	BindType string `json:"bind_type" binding:"required,oneof=menu scene permission role user"`
	BindKey  string `json:"bind_key" binding:"required,max=128"`
	Sort     int    `json:"sort"`
	Status   int    `json:"status" binding:"required,oneof=1 2"`
}
