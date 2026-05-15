package alipay

import (
	"context"
	"errors"

	paymentcore "admin_back_go/internal/platform/payment"
)

var ErrGatewayNotConfigured = errors.New("alipay: gateway not configured")

type PlatformGateway struct {
	inner *GopayGateway
}

func NewPlatformGateway(inner *GopayGateway) *PlatformGateway {
	return &PlatformGateway{inner: inner}
}

func MapChannelConfig(cfg paymentcore.ChannelConfig) ChannelConfig {
	return ChannelConfig{
		AppID:          cfg.AppID,
		PrivateKey:     cfg.PrivateKey,
		AppCertPath:    cfg.AppCertPath,
		AlipayCertPath: cfg.AlipayCertPath,
		RootCertPath:   cfg.RootCertPath,
		NotifyURL:      cfg.NotifyURL,
		IsSandbox:      cfg.IsSandbox,
	}
}

func (g *PlatformGateway) TestConfig(ctx context.Context, cfg paymentcore.ChannelConfig) error {
	if g == nil || g.inner == nil {
		return ErrGatewayNotConfigured
	}
	return g.inner.TestConfig(ctx, MapChannelConfig(cfg))
}

var _ paymentcore.Gateway = (*PlatformGateway)(nil)
