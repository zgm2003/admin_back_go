package alipay

import "context"

type ChannelConfig struct {
	AppID          string
	PrivateKey     string
	AppCertPath    string
	AlipayCertPath string
	RootCertPath   string
	NotifyURL      string
	IsSandbox      bool
}

type Gateway interface {
	TestConfig(ctx context.Context, cfg ChannelConfig) error
}
