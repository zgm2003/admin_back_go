package paynotifylog

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	ChannelArr             []dict.Option[int] `json:"channel_arr"`
	NotifyTypeArr          []dict.Option[int] `json:"notify_type_arr"`
	NotifyProcessStatusArr []dict.Option[int] `json:"notify_process_status_arr"`
}

type ListQuery struct {
	CurrentPage   int
	PageSize      int
	TransactionNo string
	Channel       *int
	NotifyType    *int
	ProcessStatus *int
	StartDate     string
	EndDate       string
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
	ID                int64  `json:"id"`
	Channel           int    `json:"channel"`
	ChannelText       string `json:"channel_text"`
	NotifyType        int    `json:"notify_type"`
	NotifyTypeText    string `json:"notify_type_text"`
	TransactionNo     string `json:"transaction_no"`
	TradeNo           string `json:"trade_no"`
	ProcessStatus     int    `json:"process_status"`
	ProcessStatusText string `json:"process_status_text"`
	ProcessMsg        string `json:"process_msg"`
	IP                string `json:"ip"`
	CreatedAt         string `json:"created_at"`
}

type DetailResponse struct {
	Log DetailLog `json:"log"`
}

type DetailLog struct {
	ID                int64          `json:"id"`
	Channel           int            `json:"channel"`
	ChannelText       string         `json:"channel_text"`
	NotifyType        int            `json:"notify_type"`
	NotifyTypeText    string         `json:"notify_type_text"`
	TransactionNo     string         `json:"transaction_no"`
	TradeNo           string         `json:"trade_no"`
	ProcessStatus     int            `json:"process_status"`
	ProcessStatusText string         `json:"process_status_text"`
	ProcessMsg        string         `json:"process_msg"`
	Headers           map[string]any `json:"headers"`
	RawData           map[string]any `json:"raw_data"`
	IP                string         `json:"ip"`
	CreatedAt         string         `json:"created_at"`
	UpdatedAt         string         `json:"updated_at"`
}

type ListRow struct {
	ID            int64
	Channel       int
	NotifyType    int
	TransactionNo string
	TradeNo       string
	ProcessStatus int
	ProcessMsg    string
	IP            string
	CreatedAt     time.Time
}

type DetailRow struct {
	ID            int64
	Channel       int
	NotifyType    int
	TransactionNo string
	TradeNo       string
	ProcessStatus int
	ProcessMsg    string
	Headers       string
	RawData       string
	IP            string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error)
}
