package notificationtask

import "time"

type statusCountRequest struct {
	Title string `form:"title" binding:"max=100"`
}

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Status      *int   `form:"status" binding:"omitempty,notification_task_status"`
	Title       string `form:"title" binding:"max=100"`
}

type createRequest struct {
	Title      string  `json:"title" binding:"required,max=100"`
	Content    string  `json:"content"`
	Type       int     `json:"type" binding:"omitempty,notification_type"`
	Level      int     `json:"level" binding:"omitempty,notification_level"`
	Link       string  `json:"link" binding:"max=500"`
	Platform   string  `json:"platform" binding:"omitempty,notification_task_platform"`
	TargetType int     `json:"target_type" binding:"required,notification_target_type"`
	TargetIDs  []int64 `json:"target_ids" binding:"omitempty,dive,gt=0"`
	SendAt     string  `json:"send_at"`
}

const timeLayout = "2006-01-02 15:04:05"

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
