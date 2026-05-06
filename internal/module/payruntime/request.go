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
