package userloginlog

type listRequest struct {
	CurrentPage  int    `form:"current_page"`
	PageSize     int    `form:"page_size"`
	UserID       int64  `form:"user_id"`
	LoginAccount string `form:"login_account"`
	LoginType    string `form:"login_type"`
	IP           string `form:"ip"`
	Platform     string `form:"platform"`
	IsSuccess    *int   `form:"is_success"`
	DateStart    string `form:"date_start"`
	DateEnd      string `form:"date_end"`
}
