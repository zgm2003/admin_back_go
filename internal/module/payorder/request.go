package payorder

type statusCountRequest struct {
	OrderNo string `form:"order_no" binding:"omitempty,max=32"`
	UserID  *int64 `form:"user_id" binding:"omitempty,min=1"`
}

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	OrderType   *int   `form:"order_type" binding:"omitempty,pay_order_type"`
	PayStatus   *int   `form:"pay_status" binding:"omitempty,pay_status"`
	OrderNo     string `form:"order_no" binding:"omitempty,max=32"`
	UserID      *int64 `form:"user_id" binding:"omitempty,min=1"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}

type remarkRequest struct {
	Remark string `json:"remark" binding:"required,max=500"`
}

type closeRequest struct {
	Reason string `json:"reason" binding:"omitempty,max=100"`
}
