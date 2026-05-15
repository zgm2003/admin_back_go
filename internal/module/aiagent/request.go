package aiagent

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=128"`
	Scene       string `form:"scene" binding:"omitempty,max=32"`
	ProviderID  uint64 `form:"provider_id" binding:"omitempty,gt=0"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type optionRequest struct {
	Scene string `form:"scene" binding:"omitempty,max=32"`
}

type mutationRequest struct {
	ProviderID   uint64   `json:"provider_id" binding:"required,gt=0"`
	Name         string   `json:"name" binding:"required,max=128"`
	ModelID      string   `json:"model_id" binding:"required,max=191"`
	Scenes       []string `json:"scenes" binding:"required,min=1,dive,required,max=32"`
	SystemPrompt string   `json:"system_prompt" binding:"omitempty,max=20000"`
	Avatar       string   `json:"avatar" binding:"omitempty,max=512"`
	Status       int      `json:"status" binding:"required,oneof=1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
