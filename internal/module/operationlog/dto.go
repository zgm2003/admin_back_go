package operationlog

import (
	"time"

	"admin_back_go/internal/shared/pagination"
)

type InitResponse struct{}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      int64
	Action      string
	DateRange   []string
}

type ListResponse struct {
	List []ListItem      `json:"list"`
	Page pagination.Page `json:"page"`
}

type ListItem struct {
	ID           int64  `json:"id"`
	UserName     string `json:"user_name"`
	UserEmail    string `json:"user_email"`
	Action       string `json:"action"`
	RequestData  string `json:"request_data"`
	ResponseData string `json:"response_data"`
	IsSuccess    int    `json:"is_success"`
	CreatedAt    string `json:"created_at"`
}

type ListRow struct {
	ID           int64
	UserID       int64
	UserName     string
	UserEmail    string
	Action       string
	RequestData  string
	ResponseData string
	IsSuccess    int
	CreatedAt    time.Time
}
