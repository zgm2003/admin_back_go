package payment

type listChannelsRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=100"`
	Name        string `form:"name" binding:"omitempty,max=128"`
	Provider    string `form:"provider" binding:"omitempty,payment_provider"`
	Status      int    `form:"status" binding:"omitempty,common_status"`
}

type channelMutationRequest struct {
	Code               string   `json:"code" binding:"required,max=64"`
	Name               string   `json:"name" binding:"required,max=128"`
	Provider           string   `json:"provider" binding:"required,payment_provider"`
	SupportedMethods   []string `json:"supported_methods" binding:"required,min=1,dive,payment_method"`
	AppID              string   `json:"app_id" binding:"required,max=64"`
	MerchantID         string   `json:"merchant_id" binding:"omitempty,max=64"`
	NotifyURL          string   `json:"notify_url" binding:"required,max=512"`
	ReturnURL          string   `json:"return_url" binding:"omitempty,max=512"`
	PrivateKey         string   `json:"private_key" binding:"omitempty"`
	AppCertPath        string   `json:"app_cert_path" binding:"required,max=512"`
	AlipayCertPath     string   `json:"alipay_cert_path" binding:"required,max=512"`
	AlipayRootCertPath string   `json:"alipay_root_cert_path" binding:"required,max=512"`
	IsSandbox          int      `json:"is_sandbox" binding:"required,common_yes_no"`
	Status             int      `json:"status" binding:"required,common_status"`
	Remark             string   `json:"remark" binding:"omitempty,max=255"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type orderListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=100"`
	OrderNo     string `form:"order_no" binding:"omitempty,max=64"`
	UserID      int64  `form:"user_id" binding:"omitempty,min=1"`
	Status      int    `form:"status" binding:"omitempty,payment_order_status"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}

type createOrderRequest struct {
	ChannelID    int64  `json:"channel_id" binding:"required,min=1"`
	PayMethod    string `json:"pay_method" binding:"required,payment_method"`
	Subject      string `json:"subject" binding:"required,max=128"`
	AmountCents  int64  `json:"amount_cents" binding:"required,min=1"`
	ReturnURL    string `json:"return_url" binding:"omitempty,max=512"`
	BusinessType string `json:"business_type" binding:"omitempty,max=64"`
	BusinessRef  string `json:"business_ref" binding:"omitempty,max=128"`
}

type payOrderRequest struct {
	ReturnURL string `json:"return_url" binding:"omitempty,max=512"`
}

type eventListRequest struct {
	CurrentPage   int    `form:"current_page" binding:"required,min=1"`
	PageSize      int    `form:"page_size" binding:"required,min=1,max=100"`
	OrderNo       string `form:"order_no" binding:"omitempty,max=64"`
	OutTradeNo    string `form:"out_trade_no" binding:"omitempty,max=64"`
	EventType     string `form:"event_type" binding:"omitempty,payment_event_type"`
	ProcessStatus int    `form:"process_status" binding:"omitempty,payment_event_process_status"`
}
