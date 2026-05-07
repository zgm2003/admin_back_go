package payreconcile

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Channel     *int   `form:"channel" binding:"omitempty,pay_channel"`
	Status      *int   `form:"status" binding:"omitempty,pay_reconcile_status"`
	BillType    *int   `form:"bill_type" binding:"omitempty,pay_reconcile_bill_type"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}
