package payment

type listOrdersRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Keyword     string `form:"keyword" binding:"max=128"`
	ConfigCode  string `form:"config_code" binding:"max=64"`
	Provider    string `form:"provider" binding:"omitempty,oneof=alipay"`
	PayMethod   string `form:"pay_method" binding:"omitempty,oneof=web h5"`
	Status      string `form:"status" binding:"omitempty,oneof=pending paying paid closed failed"`
	DateStart   string `form:"date_start" binding:"omitempty,max=20"`
	DateEnd     string `form:"date_end" binding:"omitempty,max=20"`
}

type createOrderRequest struct {
	ConfigCode    string `json:"config_code" binding:"required,max=64"`
	PayMethod     string `json:"pay_method" binding:"required,oneof=web h5"`
	Subject       string `json:"subject" binding:"required,max=128"`
	AmountCents   int64  `json:"amount_cents" binding:"required,min=1"`
	ReturnURL     string `json:"return_url" binding:"omitempty,max=512"`
	ExpireMinutes int    `json:"expire_minutes" binding:"omitempty,min=1,max=1440"`
}
