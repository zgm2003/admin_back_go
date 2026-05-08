package aiapp

type listRequest struct {
	CurrentPage        int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize           int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name               string `form:"name" binding:"max=128"`
	Code               string `form:"code" binding:"max=128"`
	AppType            string `form:"app_type" binding:"omitempty,oneof=chat workflow completion agent"`
	EngineConnectionID uint64 `form:"engine_connection_id" binding:"omitempty,gt=0"`
	Status             *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	EngineConnectionID  uint64         `json:"engine_connection_id" binding:"required,gt=0"`
	Name                string         `json:"name" binding:"required,max=128"`
	Code                string         `json:"code" binding:"required,max=128"`
	AppType             string         `json:"app_type" binding:"required,oneof=chat workflow completion agent"`
	EngineAppID         string         `json:"engine_app_id" binding:"omitempty,max=128"`
	EngineAppAPIKey     string         `json:"engine_app_api_key" binding:"omitempty,max=4096"`
	DefaultResponseMode string         `json:"default_response_mode" binding:"required,oneof=streaming blocking"`
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
