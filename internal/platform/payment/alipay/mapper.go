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
		AlipayCertPath: cfg.PlatformCertPath,
		RootCertPath:   cfg.RootCertPath,
		NotifyURL:      cfg.NotifyURL,
		IsSandbox:      cfg.IsSandbox,
	}
}

func MapPayInput(input paymentcore.PayInput) PayInput {
	return PayInput{
		Method:      input.Method,
		OutTradeNo:  input.OutTradeNo,
		Subject:     input.Subject,
		AmountCents: input.AmountCents,
		ReturnURL:   input.ReturnURL,
		ExpiredAt:   input.ExpiredAt,
	}
}

func (g *PlatformGateway) TestConfig(ctx context.Context, cfg paymentcore.ChannelConfig) error {
	if g == nil || g.inner == nil {
		return ErrGatewayNotConfigured
	}
	return g.inner.TestConfig(ctx, MapChannelConfig(cfg))
}

func (g *PlatformGateway) Pay(ctx context.Context, cfg paymentcore.ChannelConfig, input paymentcore.PayInput) (*paymentcore.PayResult, error) {
	if g == nil || g.inner == nil {
		return nil, ErrGatewayNotConfigured
	}
	result, err := g.inner.Pay(ctx, MapChannelConfig(cfg), MapPayInput(input))
	if err != nil {
		return nil, err
	}
	return &paymentcore.PayResult{PayURL: result.PayURL}, nil
}

func (g *PlatformGateway) Query(ctx context.Context, cfg paymentcore.ChannelConfig, outTradeNo string) (*paymentcore.QueryResult, error) {
	if g == nil || g.inner == nil {
		return nil, ErrGatewayNotConfigured
	}
	result, err := g.inner.Query(ctx, MapChannelConfig(cfg), outTradeNo)
	if err != nil {
		return nil, err
	}
	return &paymentcore.QueryResult{
		TradeNo: result.TradeNo,
		Status:  result.Status,
		PaidAt:  result.PaidAt,
	}, nil
}

func (g *PlatformGateway) Close(ctx context.Context, cfg paymentcore.ChannelConfig, outTradeNo string) error {
	if g == nil || g.inner == nil {
		return ErrGatewayNotConfigured
	}
	return g.inner.Close(ctx, MapChannelConfig(cfg), outTradeNo)
}

var _ paymentcore.Gateway = (*PlatformGateway)(nil)
