package aimessage

type listRequest struct {
	CurrentPage int  `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int  `form:"page_size" binding:"omitempty,min=1,max=50"`
	Role        *int `form:"role" binding:"omitempty,oneof=1 2 3"`
}

type contentRequest struct {
	Content string `json:"content" binding:"required"`
}

type feedbackRequest struct {
	Feedback *int `json:"feedback" binding:"omitempty,oneof=1 2"`
}

type batchDeleteRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,max=100,dive,min=1"`
}
