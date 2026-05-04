package queuemonitor

type failedListRequest struct {
	Queue       string `form:"queue" binding:"required,max=100"`
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
}
