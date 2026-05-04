package operationlog

import "time"

type InitResponse struct{}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      int64
	Action      string
	DateRange   []string
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
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
