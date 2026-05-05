package paytransaction

type listRequest struct {
	CurrentPage   int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize      int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	OrderNo       string `form:"order_no" binding:"omitempty,max=32"`
	TransactionNo string `form:"transaction_no" binding:"omitempty,max=64"`
	UserID        *int64 `form:"user_id" binding:"omitempty,min=1"`
	Channel       *int   `form:"channel" binding:"omitempty,pay_channel"`
	Status        *int   `form:"status" binding:"omitempty,pay_txn_status"`
	StartDate     string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate       string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}
