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
		ChannelID:      cfg.ChannelID,
		AppID:          cfg.AppID,
		PrivateKey:     cfg.PrivateKey,
		AppCertPath:    cfg.AppCertPath,
		AlipayCertPath: cfg.AlipayCertPath,
		RootCertPath:   cfg.RootCertPath,
		NotifyURL:      cfg.NotifyURL,
		IsSandbox:      cfg.IsSandbox,
	}
}

func MapCreateRequest(req paymentcore.CreatePayRequest) CreateRequest {
	return CreateRequest{
		OutTradeNo:  req.OutTradeNo,
		Subject:     req.Subject,
		AmountCents: int(req.AmountCents),
		PayMethod:   req.PayMethod,
		ReturnURL:   req.ReturnURL,
	}
}

func MapCreateResult(result *CreateResponse) *paymentcore.CreatePayResult {
	if result == nil {
		return nil
	}
	return &paymentcore.CreatePayResult{
		Mode:    result.Mode,
		Content: result.Content,
		Raw:     result.Raw,
	}
}

func MapQueryResult(result *QueryResult) *paymentcore.QueryResult {
	if result == nil {
		return nil
	}
	return &paymentcore.QueryResult{
		OutTradeNo:  result.OutTradeNo,
		TradeNo:     result.TradeNo,
		TradeStatus: result.TradeStatus,
		AmountCents: int64(result.TotalAmountCents),
		AppID:       result.AppID,
		Raw:         result.Raw,
	}
}

func MapNotifyResult(result *NotifyResult) *paymentcore.NotifyResult {
	if result == nil {
		return nil
	}
	return &paymentcore.NotifyResult{
		OutTradeNo:  result.OutTradeNo,
		TradeNo:     result.TradeNo,
		TradeStatus: result.TradeStatus,
		AmountCents: int64(result.TotalAmountCents),
		AppID:       result.AppID,
		Raw:         result.Raw,
	}
}

func (g *PlatformGateway) CreatePagePay(ctx context.Context, cfg paymentcore.ChannelConfig, req paymentcore.CreatePayRequest) (*paymentcore.CreatePayResult, error) {
	if g == nil || g.inner == nil {
		return nil, ErrGatewayNotConfigured
	}
	result, err := g.inner.Create(ctx, MapChannelConfig(cfg), MapCreateRequest(req))
	if err != nil {
		return nil, err
	}
	return MapCreateResult(result), nil
}

func (g *PlatformGateway) Query(ctx context.Context, cfg paymentcore.ChannelConfig, outTradeNo string) (*paymentcore.QueryResult, error) {
	if g == nil || g.inner == nil {
		return nil, ErrGatewayNotConfigured
	}
	result, err := g.inner.Query(ctx, MapChannelConfig(cfg), QueryRequest{OutTradeNo: outTradeNo})
	if err != nil {
		return nil, err
	}
	return MapQueryResult(result), nil
}

func (g *PlatformGateway) VerifyNotify(ctx context.Context, cfg paymentcore.ChannelConfig, form map[string]string) (*paymentcore.NotifyResult, error) {
	if g == nil || g.inner == nil {
		return nil, ErrGatewayNotConfigured
	}
	result, err := g.inner.VerifyNotify(ctx, MapChannelConfig(cfg), NotifyRequest{Form: form})
	if err != nil {
		return nil, err
	}
	return MapNotifyResult(result), nil
}

func (g *PlatformGateway) Close(ctx context.Context, cfg paymentcore.ChannelConfig, outTradeNo string) error {
	if g == nil || g.inner == nil {
		return ErrGatewayNotConfigured
	}
	return g.inner.Close(ctx, MapChannelConfig(cfg), CloseRequest{OutTradeNo: outTradeNo})
}

func (g *PlatformGateway) SuccessBody() string {
	if g == nil || g.inner == nil {
		return "success"
	}
	return g.inner.SuccessBody()
}

func (g *PlatformGateway) FailureBody() string {
	if g == nil || g.inner == nil {
		return "fail"
	}
	return g.inner.FailureBody()
}

var _ paymentcore.Gateway = (*PlatformGateway)(nil)
