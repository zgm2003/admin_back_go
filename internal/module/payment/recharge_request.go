package payment

type listRechargesRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword     string `form:"keyword" binding:"omitempty,max=128"`
	Status      string `form:"status" binding:"omitempty,oneof=pending paying paid credited closed failed"`
	DateStart   string `form:"date_start" binding:"omitempty,max=32"`
	DateEnd     string `form:"date_end" binding:"omitempty,max=32"`
}

type createRechargeRequest struct {
	PackageCode string `json:"package_code" binding:"required,max=64"`
	PayMethod   string `json:"pay_method" binding:"required,oneof=web h5"`
	ReturnURL   string `json:"return_url" binding:"required,max=512"`
}
