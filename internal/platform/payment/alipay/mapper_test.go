package alipay

import (
	"context"
	"errors"
	"testing"

	paymentcore "admin_back_go/internal/platform/payment"
)

func TestMapChannelConfig(t *testing.T) {
	cfg := MapChannelConfig(paymentcore.ChannelConfig{
		AppID:            "app",
		PrivateKey:       "private",
		AppCertPath:      "app.crt",
		PlatformCertPath: "alipay.crt",
		RootCertPath:     "root.crt",
		NotifyURL:        "https://notify",
		IsSandbox:        true,
	})
	if cfg.AppID != "app" || cfg.PrivateKey != "private" || !cfg.IsSandbox {
		t.Fatalf("unexpected mapped config: %#v", cfg)
	}
	if cfg.AppCertPath != "app.crt" || cfg.AlipayCertPath != "alipay.crt" || cfg.RootCertPath != "root.crt" || cfg.NotifyURL != "https://notify" {
		t.Fatalf("unexpected mapped cert/url fields: %#v", cfg)
	}
}

func TestPlatformGatewayNilInner(t *testing.T) {
	var gateway *PlatformGateway
	if err := gateway.TestConfig(context.Background(), paymentcore.ChannelConfig{}); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil gateway, got %v", err)
	}
	if _, err := gateway.Pay(context.Background(), paymentcore.ChannelConfig{}, paymentcore.PayInput{}); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil gateway Pay, got %v", err)
	}
	if _, err := gateway.Query(context.Background(), paymentcore.ChannelConfig{}, "out"); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil gateway Query, got %v", err)
	}
	if err := gateway.Close(context.Background(), paymentcore.ChannelConfig{}, "out"); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil gateway Close, got %v", err)
	}

	gateway = NewPlatformGateway(nil)
	if err := gateway.TestConfig(context.Background(), paymentcore.ChannelConfig{}); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil inner, got %v", err)
	}
	if _, err := gateway.Pay(context.Background(), paymentcore.ChannelConfig{}, paymentcore.PayInput{}); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil inner Pay, got %v", err)
	}
	if _, err := gateway.Query(context.Background(), paymentcore.ChannelConfig{}, "out"); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil inner Query, got %v", err)
	}
	if err := gateway.Close(context.Background(), paymentcore.ChannelConfig{}, "out"); !errors.Is(err, ErrGatewayNotConfigured) {
		t.Fatalf("expected ErrGatewayNotConfigured from nil inner Close, got %v", err)
	}
}
