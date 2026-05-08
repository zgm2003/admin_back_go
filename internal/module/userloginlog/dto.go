package userloginlog

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
}

type Option[T string] = dict.Option[T]

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	PlatformArr  []Option[string] `json:"platformArr"`
	LoginTypeArr []Option[string] `json:"login_type_arr"`
}

type ListQuery struct {
	CurrentPage  int
	PageSize     int
	UserID       int64
	LoginAccount string
	LoginType    string
	IP           string
	Platform     string
	IsSuccess    *int
	DateStart    string
	DateEnd      string
	CreatedStart string
	CreatedEnd   string
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
	ID            int64  `json:"id"`
	UserID        *int64 `json:"user_id"`
	UserName      string `json:"user_name"`
	LoginAccount  string `json:"login_account"`
	LoginType     string `json:"login_type"`
	LoginTypeName string `json:"login_type_name"`
	Platform      string `json:"platform"`
	PlatformName  string `json:"platform_name"`
	IP            string `json:"ip"`
	UserAgent     string `json:"ua"`
	IsSuccess     int    `json:"is_success"`
	Reason        string `json:"reason"`
	CreatedAt     string `json:"created_at"`
}

type ListRow struct {
	ID           int64
	UserID       *int64
	Username     string
	LoginAccount string
	LoginType    string
	Platform     string
	IP           string
	UserAgent    string
	IsSuccess    int
	Reason       string
	CreatedAt    time.Time
}
