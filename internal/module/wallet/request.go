package wallet

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	UserID      *int64 `form:"user_id" binding:"omitempty,min=1"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}

type transactionListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	UserID      *int64 `form:"user_id" binding:"omitempty,min=1"`
	Type        *int   `form:"type" binding:"omitempty,wallet_type"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}

type createAdjustmentRequest struct {
	UserID         int64  `json:"user_id" binding:"required,min=1"`
	Delta          *int   `json:"delta" binding:"required"`
	Reason         string `json:"reason" binding:"required,max=255"`
	IdempotencyKey string `json:"idempotency_key" binding:"required,min=8,max=50"`
}
