package aiengine

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=128"`
	EngineType  string `form:"engine_type" binding:"omitempty,oneof=openai"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name              string            `json:"name" binding:"required,max=128"`
	Driver            string            `json:"driver" binding:"omitempty,oneof=openai"`
	EngineType        string            `json:"engine_type" binding:"omitempty,oneof=openai"`
	BaseURL           string            `json:"base_url" binding:"omitempty,max=512"`
	APIKey            string            `json:"api_key" binding:"omitempty,max=4096"`
	WorkspaceID       string            `json:"workspace_id" binding:"omitempty,max=128"`
	ModelIDs          []string          `json:"model_ids" binding:"required,min=1,dive,required,max=191"`
	DefaultModelID    string            `json:"default_model_id" binding:"required,max=191"`
	ModelDisplayNames map[string]string `json:"model_display_names" binding:"omitempty"`
	Status            int               `json:"status" binding:"required,oneof=1 2"`
}

type modelOptionsRequest struct {
	Driver     string `json:"driver" binding:"omitempty,oneof=openai"`
	EngineType string `json:"engine_type" binding:"omitempty,oneof=openai"`
	BaseURL    string `json:"base_url" binding:"omitempty,max=512"`
	APIKey     string `json:"api_key" binding:"required,max=4096"`
}

type updateModelsRequest struct {
	ModelIDs          []string          `json:"model_ids" binding:"required,min=1,dive,required,max=191"`
	DefaultModelID    string            `json:"default_model_id" binding:"required,max=191"`
	ModelDisplayNames map[string]string `json:"model_display_names" binding:"omitempty"`
	Statuses          map[string]int    `json:"statuses" binding:"omitempty"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
