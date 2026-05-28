package admin

type transactionListRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	Direction   string `form:"direction"`
	SourceType  string `form:"source_type"`
	DateStart   string `form:"date_start"`
	DateEnd     string `form:"date_end"`
	UserID      int64  `form:"user_id"`
}

type walletUserListRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	UserID      int64  `form:"user_id"`
}

type consumeRequest struct {
	AmountCents int64  `json:"amount_cents" binding:"required,min=1"`
	SourceID    int64  `json:"source_id" binding:"required,min=1"`
	Remark      string `json:"remark" binding:"omitempty,max=255"`
}
