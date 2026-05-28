package alipay

import (
	"context"
	"net/url"
	"time"
)

type ChannelConfig struct {
	AppID          string
	PrivateKey     string
	AppCertPath    string
	AlipayCertPath string
	RootCertPath   string
	NotifyURL      string
	IsSandbox      bool
}

type PayInput struct {
	Method      string
	OutTradeNo  string
	Subject     string
	AmountCents int64
	ReturnURL   string
	ExpiredAt   time.Time
}

type PayResult struct {
	PayURL string
}

type QueryResult struct {
	TradeNo string
	Status  string
	PaidAt  *time.Time
}

type NotifyPayload struct {
	NotifyID         string
	OutTradeNo       string
	TradeNo          string
	TradeStatus      string
	AppID            string
	TotalAmountCents int64
	Raw              map[string]string
}

type Gateway interface {
	TestConfig(ctx context.Context, cfg ChannelConfig) error
	Pay(ctx context.Context, cfg ChannelConfig, in PayInput) (*PayResult, error)
	Query(ctx context.Context, cfg ChannelConfig, outTradeNo string) (*QueryResult, error)
	Close(ctx context.Context, cfg ChannelConfig, outTradeNo string) error
	VerifyNotify(ctx context.Context, cfg ChannelConfig, form url.Values) (*NotifyPayload, error)
}
