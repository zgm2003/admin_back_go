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

type NumberGenerator interface {
	Next(ctx context.Context, prefix string) (string, error)
}

type HTTPService interface {
	CreateRechargeOrder(ctx context.Context, userID int64, input RechargeOrderCreateInput) (*RechargeOrderCreateResponse, *apperror.Error)
	CreatePayAttempt(ctx context.Context, userID int64, orderNo string, input PayAttemptCreateInput) (*PayAttemptCreateResponse, *apperror.Error)
	HandleAlipayNotify(ctx context.Context, input AlipayNotifyInput) (string, *apperror.Error)
}
