package payorder

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
	ChannelArr        []dict.Option[int]    `json:"channel_arr"`
	PayMethodArr      []dict.Option[string] `json:"pay_method_arr"`
	OrderTypeArr      []dict.Option[int]    `json:"order_type_arr"`
	PayStatusArr      []dict.Option[int]    `json:"pay_status_arr"`
	BizStatusArr      []dict.Option[int]    `json:"biz_status_arr"`
	RechargePresetArr []dict.Option[int]    `json:"recharge_preset_arr"`
}

type StatusCountQuery struct {
	OrderNo string
	UserID  *int64
}

type StatusCountItem struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Count int64  `json:"count"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	OrderType   *int
	PayStatus   *int
	OrderNo     string
	UserID      *int64
	StartDate   string
	EndDate     string
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
	ID             int64   `json:"id"`
	OrderNo        string  `json:"order_no"`
	UserID         int64   `json:"user_id"`
	UserName       string  `json:"user_name"`
	UserEmail      string  `json:"user_email"`
	OrderType      int     `json:"order_type"`
	OrderTypeText  string  `json:"order_type_text"`
	Title          string  `json:"title"`
	TotalAmount    int     `json:"total_amount"`
	DiscountAmount int     `json:"discount_amount"`
	PayAmount      int     `json:"pay_amount"`
	PayStatus      int     `json:"pay_status"`
	PayStatusText  string  `json:"pay_status_text"`
	BizStatus      int     `json:"biz_status"`
	BizStatusText  string  `json:"biz_status_text"`
	AdminRemark    string  `json:"admin_remark"`
	PayTime        *string `json:"pay_time"`
	CreatedAt      string  `json:"created_at"`
}

type DetailResponse struct {
	Order DetailOrder `json:"order"`
	Items []Item      `json:"items"`
}

type DetailOrder struct {
	ID                   int64                  `json:"id"`
	OrderNo              string                 `json:"order_no"`
	UserID               int64                  `json:"user_id"`
	UserName             string                 `json:"user_name"`
	UserEmail            string                 `json:"user_email"`
	OrderType            int                    `json:"order_type"`
	OrderTypeText        string                 `json:"order_type_text"`
	BizType              string                 `json:"biz_type"`
	BizID                int64                  `json:"biz_id"`
	Title                string                 `json:"title"`
	TotalAmount          int                    `json:"total_amount"`
	DiscountAmount       int                    `json:"discount_amount"`
	PayAmount            int                    `json:"pay_amount"`
	PayStatus            int                    `json:"pay_status"`
	PayStatusText        string                 `json:"pay_status_text"`
	BizStatus            int                    `json:"biz_status"`
	BizStatusText        string                 `json:"biz_status_text"`
	PayTime              *string                `json:"pay_time"`
	ExpireTime           *string                `json:"expire_time"`
	CloseTime            *string                `json:"close_time"`
	CloseReason          string                 `json:"close_reason"`
	BizDoneAt            *string                `json:"biz_done_at"`
	AdminRemark          string                 `json:"admin_remark"`
	Channel              *ChannelSummary        `json:"channel"`
	PayMethod            string                 `json:"pay_method"`
	Extra                map[string]any         `json:"extra"`
	SuccessTransactionID int64                  `json:"success_transaction_id"`
	CreatedAt            string                 `json:"created_at"`
}

type ChannelSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Channel int    `json:"channel"`
}

type Item struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Price    int    `json:"price"`
	Quantity int    `json:"quantity"`
	Amount   int    `json:"amount"`
}

type RemarkInput struct {
	Remark string
}

type CloseInput struct {
	Reason string
	Now    time.Time
}

type ListRow struct {
	ID             int64
	OrderNo        string
	UserID         int64
	UserName       string
	UserEmail      string
	OrderType      int
	Title          string
	TotalAmount    int
	DiscountAmount int
	PayAmount      int
	PayStatus      int
	BizStatus      int
	AdminRemark    string
	PayTime        *time.Time
	CreatedAt      time.Time
}

type DetailRow struct {
	ID                   int64
	OrderNo              string
	UserID               int64
	UserName             string
	UserEmail            string
	OrderType            int
	BizType              string
	BizID                int64
	Title                string
	TotalAmount          int
	DiscountAmount       int
	PayAmount            int
	PayStatus            int
	BizStatus            int
	SuccessTransactionID int64
	ChannelID            int64
	PayMethod            string
	PayTime              *time.Time
	ExpireTime           time.Time
	CloseTime            *time.Time
	BizDoneAt            *time.Time
	CloseReason          string
	Extra                string
	AdminRemark          string
	CreatedAt            time.Time
	ChannelName          string
	Channel              int
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error)
	Remark(ctx context.Context, id int64, input RemarkInput) *apperror.Error
	Close(ctx context.Context, id int64, input CloseInput) *apperror.Error
}
