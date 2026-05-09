package aimessage

type listRequest struct {
	BeforeID int64 `form:"before_id" binding:"omitempty,min=1"`
	Limit    int   `form:"limit" binding:"omitempty,min=1,max=100"`
}

type sendRequest struct {
	Content   string `json:"content" binding:"required,max=20000"`
	RequestID string `json:"request_id" binding:"required,max=80"`
}
