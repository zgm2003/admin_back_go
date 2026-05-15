package payment

import "context"

type ChannelConfig struct {
	ChannelID      int64
	AppID          string
	PrivateKey     string
	AppCertPath    string
	AlipayCertPath string
	RootCertPath   string
	NotifyURL      string
	IsSandbox      bool
}

type CreatePayRequest struct {
	OutTradeNo  string
	Subject     string
	AmountCents int64
	PayMethod   string
	ReturnURL   string
}

type CreatePayResult struct {
	Mode    string
	Content string
	Raw     map[string]any
}

type QueryResult struct {
	OutTradeNo  string
	TradeNo     string
	TradeStatus string
	AmountCents int64
	AppID       string
	Raw         map[string]any
}

func (r QueryResult) PaidStatus() bool {
	return isPaidTradeStatus(r.TradeStatus)
}

type NotifyResult struct {
	OutTradeNo  string
	TradeNo     string
	TradeStatus string
	AmountCents int64
	AppID       string
	Raw         map[string]any
}

func (r NotifyResult) PaidStatus() bool {
	return isPaidTradeStatus(r.TradeStatus)
}

type Gateway interface {
	CreatePagePay(ctx context.Context, cfg ChannelConfig, req CreatePayRequest) (*CreatePayResult, error)
	Query(ctx context.Context, cfg ChannelConfig, outTradeNo string) (*QueryResult, error)
	VerifyNotify(ctx context.Context, cfg ChannelConfig, form map[string]string) (*NotifyResult, error)
	Close(ctx context.Context, cfg ChannelConfig, outTradeNo string) error
	TestConfig(ctx context.Context, cfg ChannelConfig) error
	SuccessBody() string
	FailureBody() string
}

func isPaidTradeStatus(status string) bool {
	return status == "TRADE_SUCCESS" || status == "TRADE_FINISHED"
}
