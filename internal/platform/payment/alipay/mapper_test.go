package alipay

import (
	"errors"
	"testing"

	paymentcore "admin_back_go/internal/platform/payment"
)

func TestMapChannelConfig(t *testing.T) {
	cfg := MapChannelConfig(paymentcore.ChannelConfig{
		ChannelID:      9,
		AppID:          "app",
		PrivateKey:     "private",
		AppCertPath:    "app.crt",
		AlipayCertPath: "alipay.crt",
		RootCertPath:   "root.crt",
		NotifyURL:      "https://notify",
		IsSandbox:      true,
	})
	if cfg.ChannelID != 9 || cfg.AppID != "app" || cfg.PrivateKey != "private" || !cfg.IsSandbox {
		t.Fatalf("unexpected mapped config: %#v", cfg)
	}
	if cfg.AppCertPath != "app.crt" || cfg.AlipayCertPath != "alipay.crt" || cfg.RootCertPath != "root.crt" || cfg.NotifyURL != "https://notify" {
		t.Fatalf("unexpected mapped cert/url fields: %#v", cfg)
	}
}

func TestMapCreateRequest(t *testing.T) {
	req := MapCreateRequest(paymentcore.CreatePayRequest{
		OutTradeNo:  "out",
		Subject:     "subject",
		AmountCents: 1234,
		PayMethod:   "web",
		ReturnURL:   "https://return",
	})
	if req.OutTradeNo != "out" || req.Subject != "subject" || req.AmountCents != 1234 || req.PayMethod != "web" || req.ReturnURL != "https://return" {
		t.Fatalf("unexpected mapped create request: %#v", req)
	}
}

func TestGatewayResultMapping(t *testing.T) {
	result := MapQueryResult(&QueryResult{OutTradeNo: "out", TradeNo: "trade", TradeStatus: "TRADE_SUCCESS", TotalAmountCents: 1234, AppID: "app"})
	if result.OutTradeNo != "out" || result.AmountCents != 1234 || result.PaidStatus() != true {
		t.Fatalf("unexpected mapped query result: %#v", result)
	}

	notify := MapNotifyResult(&NotifyResult{OutTradeNo: "out", TradeNo: "trade", TradeStatus: "TRADE_FINISHED", TotalAmountCents: 5678, AppID: "app"})
	if notify.TradeNo != "trade" || notify.AmountCents != 5678 || notify.PaidStatus() != true {
		t.Fatalf("unexpected mapped notify result: %#v", notify)
	}

	create := MapCreateResult(&CreateResponse{Mode: "external", Content: "html", Raw: map[string]any{"content": "html"}})
	if create.Mode != "external" || create.Content != "html" || create.Raw["content"] != "html" {
		t.Fatalf("unexpected mapped create result: %#v", create)
	}
}

func TestNilResultMapping(t *testing.T) {
	if MapCreateResult(nil) != nil {
		t.Fatalf("expected nil create mapping")
	}
	if MapQueryResult(nil) != nil {
		t.Fatalf("expected nil query mapping")
	}
	if MapNotifyResult(nil) != nil {
		t.Fatalf("expected nil notify mapping")
	}
}

func TestPlatformGatewayNilInner(t *testing.T) {
	var gateway *PlatformGateway
	if _, err := gateway.CreatePagePay(nil, paymentcore.ChannelConfig{}, paymentcore.CreatePayRequest{}); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil gateway, got %v", err)
	}

	gateway = NewPlatformGateway(nil)
	if _, err := gateway.Query(nil, paymentcore.ChannelConfig{}, "out"); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from query, got %v", err)
	}
	if _, err := gateway.VerifyNotify(nil, paymentcore.ChannelConfig{}, nil); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from notify, got %v", err)
	}
	if err := gateway.Close(nil, paymentcore.ChannelConfig{}, "out"); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from close, got %v", err)
	}
	if gateway.SuccessBody() != "success" || gateway.FailureBody() != "fail" {
		t.Fatalf("unexpected nil adapter bodies: %q %q", gateway.SuccessBody(), gateway.FailureBody())
	}
}
