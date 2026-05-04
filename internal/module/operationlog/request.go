package operationlog

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	UserID      int64  `form:"user_id" binding:"omitempty,gt=0"`
	Action      string `form:"action" binding:"omitempty,max=255"`
	Date        string `form:"date" binding:"omitempty"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
