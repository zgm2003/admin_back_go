package payment

type listConfigsRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Name        string `form:"name" binding:"omitempty,max=128"`
	Provider    string `form:"provider" binding:"omitempty,payment_provider"`
	Environment string `form:"environment" binding:"omitempty,oneof=sandbox production"`
	Status      int    `form:"status" binding:"omitempty,common_status"`
}

type configMutationRequest struct {
	Provider         string   `json:"provider" binding:"required,payment_provider"`
	Code             string   `json:"code" binding:"required,max=64"`
	Name             string   `json:"name" binding:"required,max=128"`
	AppID            string   `json:"app_id" binding:"required,max=64"`
	AppPrivateKey    string   `json:"app_private_key"`
	AppCertPath      string   `json:"app_cert_path" binding:"required,max=512"`
	PlatformCertPath string   `json:"platform_cert_path" binding:"required,max=512"`
	RootCertPath     string   `json:"root_cert_path" binding:"required,max=512"`
	NotifyURL        string   `json:"notify_url" binding:"required,max=512"`
	Environment      string   `json:"environment" binding:"required,oneof=sandbox production"`
	EnabledMethods   []string `json:"enabled_methods" binding:"required,min=1"`
	Status           int      `json:"status" binding:"required,common_status"`
	Remark           string   `json:"remark" binding:"omitempty,max=255"`
}

type changeConfigStatusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
