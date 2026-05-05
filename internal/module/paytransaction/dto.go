package paytransaction

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
	ChannelArr   []dict.Option[int] `json:"channel_arr"`
	TxnStatusArr []dict.Option[int] `json:"txn_status_arr"`
}

type ListQuery struct {
	CurrentPage   int
	PageSize      int
	OrderNo       string
	TransactionNo string
	UserID        *int64
	Channel       *int
	Status        *int
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
	ID              int64   `json:"id"`
	TransactionNo   string  `json:"transaction_no"`
	OrderNo         string  `json:"order_no"`
	UserID          int64   `json:"user_id"`
	UserName        string  `json:"user_name"`
	UserEmail       string  `json:"user_email"`
	AttemptNo       int     `json:"attempt_no"`
	ChannelID       int64   `json:"channel_id"`
	Channel         int     `json:"channel"`
	ChannelText     string  `json:"channel_text"`
	PayMethod       string  `json:"pay_method"`
	PayMethodText   string  `json:"pay_method_text"`
	Amount          int     `json:"amount"`
	TradeNo         string  `json:"trade_no"`
	TradeStatus     string  `json:"trade_status"`
	Status          int     `json:"status"`
	StatusText      string  `json:"status_text"`
	PaidAt          *string `json:"paid_at"`
	CreatedAt       string  `json:"created_at"`
}

type DetailResponse struct {
	Transaction DetailTransaction `json:"transaction"`
	Channel     *ChannelSummary   `json:"channel"`
	Order       *OrderSummary     `json:"order"`
}

type DetailTransaction struct {
	ID            int64                  `json:"id"`
	TransactionNo string                 `json:"transaction_no"`
	OrderNo       string                 `json:"order_no"`
	AttemptNo     int                    `json:"attempt_no"`
	ChannelID     int64                  `json:"channel_id"`
	Channel       int                    `json:"channel"`
	ChannelText   string                 `json:"channel_text"`
	PayMethod     string                 `json:"pay_method"`
	PayMethodText string                 `json:"pay_method_text"`
	Amount        int                    `json:"amount"`
	TradeNo       string                 `json:"trade_no"`
	TradeStatus   string                 `json:"trade_status"`
	Status        int                    `json:"status"`
	StatusText    string                 `json:"status_text"`
	PaidAt        *string                `json:"paid_at"`
	ClosedAt      *string                `json:"closed_at"`
	ChannelResp   map[string]any         `json:"channel_resp"`
	RawNotify     map[string]any         `json:"raw_notify"`
	CreatedAt     string                 `json:"created_at"`
}

type ChannelSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Channel int    `json:"channel"`
}

type OrderSummary struct {
	ID        int64  `json:"id"`
	OrderNo   string `json:"order_no"`
	UserID    int64  `json:"user_id"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
	Title     string `json:"title"`
	PayAmount int    `json:"pay_amount"`
	PayStatus int    `json:"pay_status"`
}

type ListRow struct {
	ID            int64
	TransactionNo string
	OrderNo       string
	UserID        int64
	UserName      string
	UserEmail     string
	AttemptNo     int
	ChannelID     int64
	Channel       int
	PayMethod     string
	Amount        int
	TradeNo       string
	TradeStatus   string
	Status        int
	PaidAt        *time.Time
	CreatedAt     time.Time
}

type DetailRow struct {
	ID            int64
	TransactionNo string
	OrderID       int64
	OrderNo       string
	AttemptNo     int
	ChannelID     int64
	Channel       int
	PayMethod     string
	Amount        int
	TradeNo       string
	TradeStatus   string
	Status        int
	PaidAt        *time.Time
	ClosedAt      *time.Time
	ChannelResp   string
	RawNotify     string
	CreatedAt     time.Time
	PayChannelName string
	OrderUserID   int64
	OrderUserName string
	OrderUserEmail string
	OrderTitle    string
	OrderPayAmount int
	OrderPayStatus int
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error)
}
