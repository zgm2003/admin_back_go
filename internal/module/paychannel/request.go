package paychannel

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Name        string `form:"name" binding:"max=80"`
	Channel     *int   `form:"channel" binding:"omitempty,pay_channel"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Name             string   `json:"name" binding:"required,max=50"`
	Channel          int      `json:"channel" binding:"required,pay_channel"`
	SupportedMethods []string `json:"supported_methods" binding:"required,min=1,dive,pay_method"`
	MchID            string   `json:"mch_id" binding:"required,max=64"`
	AppID            string   `json:"app_id" binding:"max=64"`
	NotifyURL        string   `json:"notify_url" binding:"max=512"`
	AppPrivateKey    string   `json:"app_private_key"`
	PublicCertPath   string   `json:"public_cert_path" binding:"max=512"`
	PlatformCertPath string   `json:"platform_cert_path" binding:"max=512"`
	RootCertPath     string   `json:"root_cert_path" binding:"max=512"`
	Sort             int      `json:"sort" binding:"min=0,max=9999"`
	IsSandbox        int      `json:"is_sandbox" binding:"required,common_yes_no"`
	Status           int      `json:"status" binding:"required,common_status"`
	Remark           string   `json:"remark" binding:"max=255"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
