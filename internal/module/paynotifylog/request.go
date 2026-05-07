package paynotifylog

type listRequest struct {
	CurrentPage   int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize      int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	TransactionNo string `form:"transaction_no" binding:"omitempty,max=64"`
	Channel       *int   `form:"channel" binding:"omitempty,pay_channel"`
	NotifyType    *int   `form:"notify_type" binding:"omitempty,pay_notify_type"`
	ProcessStatus *int   `form:"process_status" binding:"omitempty,pay_notify_process_status"`
	StartDate     string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate       string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}
