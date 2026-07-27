package payment

import "admin_back_go/internal/shared/dict"

const (
	rechargeStatusPending  = "pending"
	rechargeStatusPaying   = "paying"
	rechargeStatusPaid     = "paid"
	rechargeStatusCredited = "credited"
	rechargeStatusClosed   = "closed"
	rechargeStatusFailed   = "failed"

	walletDirectionIn    = "in"
	walletSourceRecharge = "recharge"
)

type RechargePageInitResponse struct {
	Wallet        WalletSummary         `json:"wallet"`
	Packages      []RechargePackageItem `json:"packages"`
	PaymentMethod RechargePaymentMethod `json:"payment_method"`
	Dict          RechargePageInitDict  `json:"dict"`
}

type RechargePageInitDict struct {
	StatusArr []dict.Option[string] `json:"status_arr"`
}

type WalletSummary struct {
	Balance          string `json:"balance"`
	AvailableBalance string `json:"available_balance"`
	HeldAmount       string `json:"held_amount"`
	TotalRecharge    string `json:"total_recharge"`
	TotalConsume     string `json:"total_consume"`
}

type RechargePackageItem struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	AmountCents int64  `json:"amount_cents"`
	AmountText  string `json:"amount_text"`
	Badge       string `json:"badge"`
}

type RechargePaymentMethod struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Enabled  bool   `json:"enabled"`
}

type RechargeListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      int64
	Keyword     string
	Status      string
	DateStart   string
	DateEnd     string
}

type RechargeCreateInput struct {
	UserID      int64
	PackageCode string
	PayMethod   string
	ReturnURL   string
}

type RechargeListResponse struct {
	List []RechargeListItem `json:"list"`
	Page Page               `json:"page"`
}

type RechargeListItem struct {
	ID             int64  `json:"id"`
	RechargeNo     string `json:"recharge_no"`
	PaymentOrderNo string `json:"payment_order_no"`
	PackageCode    string `json:"package_code"`
	PackageName    string `json:"package_name"`
	AmountCents    int64  `json:"amount_cents"`
	AmountText     string `json:"amount_text"`
	Status         string `json:"status"`
	StatusText     string `json:"status_text"`
	PayURL         string `json:"pay_url"`
	PaidAt         string `json:"paid_at"`
	CreditedAt     string `json:"credited_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type RechargeDetail struct {
	RechargeListItem
	FailureReason string `json:"failure_reason"`
	AlipayTradeNo string `json:"alipay_trade_no"`
}

type RechargePayResponse struct {
	ID             int64  `json:"id"`
	RechargeNo     string `json:"recharge_no"`
	PaymentOrderNo string `json:"payment_order_no"`
	Status         string `json:"status"`
	PayURL         string `json:"pay_url"`
}

type RechargeStatusResponse struct {
	ID            int64         `json:"id"`
	RechargeNo    string        `json:"recharge_no"`
	Status        string        `json:"status"`
	StatusText    string        `json:"status_text"`
	Wallet        WalletSummary `json:"wallet"`
	PaidAt        string        `json:"paid_at"`
	CreditedAt    string        `json:"credited_at"`
	FailureReason string        `json:"failure_reason"`
}
