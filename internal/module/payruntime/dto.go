package payruntime

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
)

type RechargeOrderCreateInput struct {
	Amount    int
	PayMethod string
	ChannelID int64
	IP        string
}

type RechargeOrderCreateResponse struct {
	OrderID    int64  `json:"order_id"`
	OrderNo    string `json:"order_no"`
	PayAmount  int    `json:"pay_amount"`
	ExpireTime string `json:"expire_time"`
}

type PayAttemptCreateInput struct {
	PayMethod string
	ReturnURL string
}

type PayAttemptCreateResponse struct {
	TransactionNo string         `json:"transaction_no"`
	TransactionID int64          `json:"txn_id"`
	OrderNo       string         `json:"order_no"`
	PayAmount     int            `json:"pay_amount"`
	Channel       int            `json:"channel"`
	PayMethod     string         `json:"pay_method"`
	NotifyURL     string         `json:"notify_url"`
	ReturnURL     string         `json:"return_url"`
	PayData       map[string]any `json:"pay_data"`
}

type AlipayNotifyInput struct {
	Form    map[string]string
	Headers map[string]string
	IP      string
}

type CurrentUserOrderListQuery struct {
	CurrentPage int
	PageSize    int
}

type WalletBillsQuery struct {
	CurrentPage int
	PageSize    int
}

type CancelOrderInput struct {
	Reason string
	Now    time.Time
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type CurrentUserOrderListResponse struct {
	List []CurrentUserOrderItem `json:"list"`
	Page Page                   `json:"page"`
}

type CurrentUserOrderItem struct {
	ID                int64   `json:"id"`
	OrderNo           string  `json:"order_no"`
	Title             string  `json:"title"`
	PayAmount         int     `json:"pay_amount"`
	PayStatus         int     `json:"pay_status"`
	PayStatusText     string  `json:"pay_status_text"`
	BizStatus         int     `json:"biz_status"`
	BizStatusText     string  `json:"biz_status_text"`
	PayTime           *string `json:"pay_time"`
	CreatedAt         string  `json:"created_at"`
	ExpireTime        *string `json:"expire_time"`
	ChannelID         *int64  `json:"channel_id"`
	ChannelName       string  `json:"channel_name"`
	PayMethod         string  `json:"pay_method"`
	PayMethodText     string  `json:"pay_method_text"`
	TransactionNo     *string `json:"transaction_no"`
	TransactionStatus *int    `json:"transaction_status"`
}

type OrderQueryResultResponse struct {
	OrderNo     string              `json:"order_no"`
	PayStatus   int                 `json:"pay_status"`
	BizStatus   int                 `json:"biz_status"`
	PayTime     *string             `json:"pay_time"`
	Transaction *TransactionSummary `json:"transaction"`
}

type TransactionSummary struct {
	TransactionNo string `json:"transaction_no"`
	Status        int    `json:"status"`
	TradeNo       string `json:"trade_no"`
}

type WalletSummaryResponse struct {
	WalletExists  int    `json:"wallet_exists"`
	Balance       int    `json:"balance"`
	Frozen        int    `json:"frozen"`
	TotalRecharge int    `json:"total_recharge"`
	TotalConsume  int    `json:"total_consume"`
	CreatedAt     string `json:"created_at"`
}

type WalletBillsResponse struct {
	List []WalletBillItem `json:"list"`
	Page Page             `json:"page"`
}

type WalletBillItem struct {
	ID             int64  `json:"id"`
	BizActionNo    string `json:"biz_action_no"`
	Type           int    `json:"type"`
	TypeText       string `json:"type_text"`
	AvailableDelta int    `json:"available_delta"`
	FrozenDelta    int    `json:"frozen_delta"`
	BalanceBefore  int    `json:"balance_before"`
	BalanceAfter   int    `json:"balance_after"`
	Title          string `json:"title"`
	Remark         string `json:"remark"`
	OrderNo        string `json:"order_no"`
	CreatedAt      string `json:"created_at"`
}

type RechargeOrderMutation struct {
	OrderNo    string
	UserID     int64
	Amount     int
	PayMethod  string
	ChannelID  int64
	Title      string
	ExpireTime time.Time
	IP         string
	Now        time.Time
}

type RechargeOrderCreated struct {
	OrderID    int64
	OrderNo    string
	PayAmount  int
	ExpireTime time.Time
}

type TransactionMutation struct {
	TransactionNo string
	OrderID       int64
	OrderNo       string
	AttemptNo     int
	ChannelID     int64
	Channel       int
	PayMethod     string
	Amount        int
	Now           time.Time
}

type NotifyLogMutation struct {
	Channel       int
	TransactionNo string
	TradeNo       string
	Headers       map[string]string
	RawData       map[string]string
	IP            string
	Now           time.Time
}

type NotifyLogUpdate struct {
	TransactionNo  string
	TradeNo        string
	ProcessStatus  int
	ProcessMessage string
	Now            time.Time
}

type PaySuccessMutation struct {
	TransactionNo string
	TradeNo       string
	TradeStatus   string
	RawNotify     map[string]any
	PaidAt        time.Time
	FulfillNo     string
}

type PaySuccessResult struct {
	AlreadySuccess bool
	OrderID        int64
	OrderNo        string
	TransactionID  int64
	WalletBefore   int
	WalletAfter    int
}

type CloseExpiredOrderInput struct {
	Limit int
	Now   time.Time
}

type CloseExpiredOrderResult struct {
	Scanned  int
	Closed   int
	Paid     int
	Deferred int
	Skipped  int
}

type SyncPendingTransactionInput struct {
	Limit int
	Now   time.Time
}

type SyncPendingTransactionResult struct {
	Scanned  int
	Paid     int
	Unpaid   int
	Deferred int
	Skipped  int
}

type ExpiredRechargeOrder struct {
	ID      int64
	OrderNo string
}

type PendingTransaction struct {
	ID            int64
	TransactionNo string
	OrderID       int64
	OrderNo       string
	ChannelID     int64
	Channel       int
	PayMethod     string
	Amount        int
	TradeNo       string
	Status        int
	CreatedAt     time.Time
}

type CurrentUserOrderRow struct {
	ID                int64
	OrderNo           string
	Title             string
	PayAmount         int
	PayStatus         int
	BizStatus         int
	PayTime           *time.Time
	CreatedAt         time.Time
	ExpireTime        *time.Time
	ChannelID         *int64
	ChannelName       string
	PayMethod         string
	TransactionNo     *string
	TransactionStatus *int
}

type WalletSummaryRow struct {
	Exists        bool
	Balance       int
	Frozen        int
	TotalRecharge int
	TotalConsume  int
	CreatedAt     *time.Time
}

type WalletBillRow struct {
	ID             int64
	BizActionNo    string
	Type           int
	AvailableDelta int
	FrozenDelta    int
	BalanceBefore  int
	BalanceAfter   int
	Title          string
	Remark         string
	OrderNo        string
	CreatedAt      time.Time
}

type NumberGenerator interface {
	Next(ctx context.Context, prefix string) (string, error)
}

type HTTPService interface {
	CreateRechargeOrder(ctx context.Context, userID int64, input RechargeOrderCreateInput) (*RechargeOrderCreateResponse, *apperror.Error)
	CreatePayAttempt(ctx context.Context, userID int64, orderNo string, input PayAttemptCreateInput) (*PayAttemptCreateResponse, *apperror.Error)
	ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) (*CurrentUserOrderListResponse, *apperror.Error)
	QueryCurrentUserRechargeResult(ctx context.Context, userID int64, orderNo string) (*OrderQueryResultResponse, *apperror.Error)
	CancelCurrentUserRechargeOrder(ctx context.Context, userID int64, orderNo string, input CancelOrderInput) *apperror.Error
	CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryResponse, *apperror.Error)
	CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) (*WalletBillsResponse, *apperror.Error)
	HandleAlipayNotify(ctx context.Context, input AlipayNotifyInput) (string, *apperror.Error)
}
