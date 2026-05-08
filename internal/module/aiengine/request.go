package aiengine

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=128"`
	EngineType  string `form:"engine_type" binding:"omitempty,oneof=dify direct eino ragflow"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name        string `json:"name" binding:"required,max=128"`
	EngineType  string `json:"engine_type" binding:"required,oneof=dify direct eino ragflow"`
	BaseURL     string `json:"base_url" binding:"required,url,max=512"`
	APIKey      string `json:"api_key" binding:"omitempty,max=4096"`
	WorkspaceID string `json:"workspace_id" binding:"omitempty,max=128"`
	Status      int    `json:"status" binding:"required,oneof=1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
