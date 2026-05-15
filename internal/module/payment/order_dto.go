package payment

import "admin_back_go/internal/dict"

const (
	orderStatusPending = "pending"
	orderStatusPaying  = "paying"
	orderStatusPaid    = "paid"
	orderStatusClosed  = "closed"
	orderStatusFailed  = "failed"
)

type OrderInitResponse struct {
	Dict          OrderInitDict       `json:"dict"`
	ConfigOptions []OrderConfigOption `json:"config_options"`
}

type OrderInitDict struct {
	ProviderArr    []dict.Option[string] `json:"provider_arr"`
	PayMethodArr   []dict.Option[string] `json:"pay_method_arr"`
	OrderStatusArr []dict.Option[string] `json:"order_status_arr"`
}

type OrderConfigOption struct {
	Label          string   `json:"label"`
	Value          string   `json:"value"`
	Provider       string   `json:"provider"`
	EnabledMethods []string `json:"enabled_methods"`
}

type OrderListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	ConfigCode  string
	Provider    string
	PayMethod   string
	Status      string
	DateStart   string
	DateEnd     string
}

type OrderCreateInput struct {
	ConfigCode    string
	PayMethod     string
	Subject       string
	AmountCents   int64
	ReturnURL     string
	ExpireMinutes int
}

type OrderListResponse struct {
	List []OrderListItem `json:"list"`
	Page Page            `json:"page"`
}

type OrderListItem struct {
	ID            int64  `json:"id"`
	OrderNo       string `json:"order_no"`
	ConfigCode    string `json:"config_code"`
	Provider      string `json:"provider"`
	ProviderText  string `json:"provider_text"`
	PayMethod     string `json:"pay_method"`
	PayMethodText string `json:"pay_method_text"`
	Subject       string `json:"subject"`
	AmountCents   int64  `json:"amount_cents"`
	AmountText    string `json:"amount_text"`
	Status        string `json:"status"`
	StatusText    string `json:"status_text"`
	PayURL        string `json:"pay_url"`
	ExpiredAt     string `json:"expired_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type OrderDetail struct {
	OrderListItem
	ReturnURL     string `json:"return_url"`
	AlipayTradeNo string `json:"alipay_trade_no"`
	PaidAt        string `json:"paid_at"`
	ClosedAt      string `json:"closed_at"`
	FailureReason string `json:"failure_reason"`
}

type OrderCreateResponse struct {
	ID      int64  `json:"id"`
	OrderNo string `json:"order_no"`
	Status  string `json:"status"`
}

type OrderPayResponse struct {
	ID      int64  `json:"id"`
	OrderNo string `json:"order_no"`
	Status  string `json:"status"`
	PayURL  string `json:"pay_url"`
}

type OrderStatusResponse struct {
	ID            int64  `json:"id"`
	OrderNo       string `json:"order_no"`
	Status        string `json:"status"`
	StatusText    string `json:"status_text"`
	AlipayTradeNo string `json:"alipay_trade_no"`
	PaidAt        string `json:"paid_at"`
	ClosedAt      string `json:"closed_at"`
}
