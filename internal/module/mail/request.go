package mail

type saveConfigRequest struct {
	SecretID  string `json:"secret_id"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region" binding:"required,max=64"`
	Endpoint  string `json:"endpoint" binding:"omitempty,max=128"`
	FromEmail string `json:"from_email" binding:"required,email,max=255"`
	FromName  string `json:"from_name" binding:"omitempty,max=100"`
	ReplyTo   string `json:"reply_to" binding:"omitempty,email,max=255"`
	Status    int    `json:"status" binding:"required,common_status"`
}

type testRequest struct {
	ToEmail       string `json:"to_email" binding:"required,email,max=255"`
	TemplateScene string `json:"template_scene" binding:"required"`
}

type templateRequest struct {
	Scene             string            `json:"scene" binding:"required"`
	Name              string            `json:"name" binding:"required,max=100"`
	Subject           string            `json:"subject" binding:"required,max=200"`
	TencentTemplateID uint64            `json:"tencent_template_id" binding:"required"`
	Variables         []string          `json:"variables" binding:"required"`
	SampleVariables   map[string]string `json:"sample_variables" binding:"required"`
	Status            int               `json:"status" binding:"required,common_status"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type logListRequest struct {
	CurrentPage    int    `form:"current_page"`
	PageSize       int    `form:"page_size"`
	Scene          string `form:"scene"`
	Status         *int   `form:"status"`
	ToEmail        string `form:"to_email"`
	CreatedAtStart string `form:"created_at_start"`
	CreatedAtEnd   string `form:"created_at_end"`
}

type deleteLogsRequest struct {
	IDs []uint64 `json:"ids" binding:"required"`
}
