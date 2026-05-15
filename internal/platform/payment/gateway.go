package payment

import "context"

type ChannelConfig struct {
	Provider         string
	AppID            string
	PrivateKey       string
	AppCertPath      string
	PlatformCertPath string
	RootCertPath     string
	NotifyURL        string
	IsSandbox        bool
}

type Gateway interface {
	TestConfig(ctx context.Context, cfg ChannelConfig) error
}
