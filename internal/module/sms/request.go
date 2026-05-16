package sms

type saveConfigRequest struct {
	SecretID             string `json:"secret_id"`
	SecretKey            string `json:"secret_key"`
	SmsSdkAppID          string `json:"sms_sdk_app_id" binding:"required"`
	SignName             string `json:"sign_name" binding:"required"`
	Region               string `json:"region"`
	Endpoint             string `json:"endpoint"`
	Status               int    `json:"status" binding:"required"`
	VerifyCodeTTLMinutes int    `json:"verify_code_ttl_minutes" binding:"required"`
}

type testRequest struct {
	ToPhone       string `json:"to_phone" binding:"required"`
	TemplateScene string `json:"template_scene" binding:"required"`
}

type templateRequest struct {
	Scene             string            `json:"scene" binding:"required"`
	Name              string            `json:"name" binding:"required"`
	TencentTemplateID string            `json:"tencent_template_id" binding:"required"`
	Variables         []string          `json:"variables" binding:"required"`
	SampleVariables   map[string]string `json:"sample_variables" binding:"required"`
	Status            int               `json:"status" binding:"required"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required"`
}

type logListRequest struct {
	CurrentPage    int    `form:"current_page"`
	PageSize       int    `form:"page_size"`
	Scene          string `form:"scene"`
	Status         *int   `form:"status"`
	ToPhone        string `form:"to_phone"`
	CreatedAtStart string `form:"created_at_start"`
	CreatedAtEnd   string `form:"created_at_end"`
}

type deleteLogsRequest struct {
	IDs []uint64 `json:"ids" binding:"required"`
}
