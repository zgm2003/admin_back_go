package payment

type listConfigsRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Name        string `form:"name" binding:"omitempty,max=128"`
	Environment string `form:"environment" binding:"omitempty,oneof=sandbox production"`
	Status      int    `form:"status" binding:"omitempty,common_status"`
}

type configMutationRequest struct {
	Code               string   `json:"code" binding:"required,max=64"`
	Name               string   `json:"name" binding:"required,max=128"`
	AppID              string   `json:"app_id" binding:"required,max=64"`
	AppPrivateKey      string   `json:"app_private_key"`
	AppCertPath        string   `json:"app_cert_path" binding:"required,max=512"`
	AlipayCertPath     string   `json:"alipay_cert_path" binding:"required,max=512"`
	AlipayRootCertPath string   `json:"alipay_root_cert_path" binding:"required,max=512"`
	NotifyURL          string   `json:"notify_url" binding:"required,max=512"`
	ReturnURL          string   `json:"return_url" binding:"omitempty,max=512"`
	Environment        string   `json:"environment" binding:"required,oneof=sandbox production"`
	EnabledMethods     []string `json:"enabled_methods" binding:"required,min=1"`
	Status             int      `json:"status" binding:"required,common_status"`
	Remark             string   `json:"remark" binding:"omitempty,max=255"`
}

type changeConfigStatusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
