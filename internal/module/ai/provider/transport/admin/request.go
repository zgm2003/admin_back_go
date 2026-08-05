package admin

import aiprovidermodule "admin_back_go/internal/module/ai/provider"

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=128"`
	EngineType  string `form:"engine_type" binding:"omitempty,oneof=openai"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name              string                                `json:"name" binding:"required,max=128"`
	EngineType        string                                `json:"engine_type" binding:"required,oneof=openai"`
	BaseURL           string                                `json:"base_url" binding:"omitempty,max=512"`
	APIKey            string                                `json:"api_key" binding:"omitempty,max=4096"`
	APIProtocol       string                                `json:"api_protocol" binding:"required,oneof=chat_completions responses"`
	ModelIDs          []string                              `json:"model_ids" binding:"omitempty,min=1,dive,required,max=191"`
	Models            []aiprovidermodule.ProviderModelInput `json:"models" binding:"omitempty,min=1,dive"`
	ModelDisplayNames map[string]string                     `json:"model_display_names" binding:"omitempty"`
	Statuses          map[string]int                        `json:"statuses" binding:"omitempty"`
	Status            int                                   `json:"status" binding:"required,oneof=1 2"`
}

type modelOptionsRequest struct {
	EngineType string `json:"engine_type" binding:"required,oneof=openai"`
	BaseURL    string `json:"base_url" binding:"omitempty,max=512"`
	APIKey     string `json:"api_key" binding:"required,max=4096"`
}

type updateModelsRequest struct {
	ModelIDs          []string                              `json:"model_ids" binding:"omitempty,min=1,dive,required,max=191"`
	Models            []aiprovidermodule.ProviderModelInput `json:"models" binding:"omitempty,min=1,dive"`
	ModelDisplayNames map[string]string                     `json:"model_display_names" binding:"omitempty"`
	Statuses          map[string]int                        `json:"statuses" binding:"omitempty"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
