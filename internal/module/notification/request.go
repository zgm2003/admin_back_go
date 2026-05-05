package notification

import "time"

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Keyword     string `form:"keyword" binding:"max=100"`
	Type        *int   `form:"type" binding:"omitempty,notification_type"`
	Level       *int   `form:"level" binding:"omitempty,notification_level"`
	IsRead      *int   `form:"is_read" binding:"omitempty,common_yes_no"`
}

type readBatchRequest struct {
	IDs []int64 `json:"ids" binding:"omitempty,dive,gt=0"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

const timeLayout = "2006-01-02 15:04:05"

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
