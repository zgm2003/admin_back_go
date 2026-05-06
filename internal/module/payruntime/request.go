package payruntime

type rechargeOrderCreateRequest struct {
	Amount    int    `json:"amount" binding:"required,min=1"`
	PayMethod string `json:"pay_method" binding:"required,pay_method"`
	ChannelID int64  `json:"channel_id" binding:"required,min=1"`
}

type payAttemptCreateRequest struct {
	PayMethod string `json:"pay_method" binding:"omitempty,pay_method"`
	ReturnURL string `json:"return_url" binding:"omitempty,max=512"`
}

type currentUserListRequest struct {
	CurrentPage int `form:"current_page" binding:"required,min=1"`
	PageSize    int `form:"page_size" binding:"required,min=1,max=50"`
}

type walletBillsRequest struct {
	CurrentPage int `form:"current_page" binding:"required,min=1"`
	PageSize    int `form:"page_size" binding:"required,min=1,max=50"`
}

type cancelOrderRequest struct {
	Reason string `json:"reason" binding:"omitempty,max=100"`
}
