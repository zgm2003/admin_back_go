package alipay

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

type CreateRequest struct {
	OutTradeNo  string
	Subject     string
	AmountCents int
	PayMethod   string
	ReturnURL   string
}

type CreateResponse struct {
	Mode    string
	Content string
	Raw     map[string]any
}

type NotifyRequest struct {
	Form map[string]string
}

type NotifyResult struct {
	OutTradeNo       string
	TradeNo          string
	TradeStatus      string
	TotalAmountCents int
	AppID            string
	Raw              map[string]any
}

type QueryRequest struct {
	OutTradeNo string
	TradeNo    string
}

type QueryResult struct {
	OutTradeNo       string
	TradeNo          string
	TradeStatus      string
	TotalAmountCents int
	AppID            string
	Raw              map[string]any
}

type CloseRequest struct {
	OutTradeNo string
	TradeNo    string
}

type Gateway interface {
	Create(ctx context.Context, cfg ChannelConfig, req CreateRequest) (*CreateResponse, error)
	Query(ctx context.Context, cfg ChannelConfig, req QueryRequest) (*QueryResult, error)
	Close(ctx context.Context, cfg ChannelConfig, req CloseRequest) error
	VerifyNotify(ctx context.Context, cfg ChannelConfig, req NotifyRequest) (*NotifyResult, error)
	SuccessBody() string
	FailureBody() string
}
