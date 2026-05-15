package alipay

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gopayalipay "github.com/go-pay/gopay/alipay"
)

type GopayGateway struct{}

func NewGopayGateway() *GopayGateway {
	return &GopayGateway{}
}

func (g *GopayGateway) TestConfig(ctx context.Context, cfg ChannelConfig) error {
	_ = ctx
	_, err := newClient(cfg)
	return err
}

func newClient(cfg ChannelConfig) (*gopayalipay.Client, error) {
	if err := validateChannelConfig(cfg); err != nil {
		return nil, err
	}
	client, err := gopayalipay.NewClient(strings.TrimSpace(cfg.AppID), strings.TrimSpace(cfg.PrivateKey), !cfg.IsSandbox)
	if err != nil {
		return nil, fmt.Errorf("alipay: new client: %w", err)
	}
	client.SetNotifyUrl(strings.TrimSpace(cfg.NotifyURL))
	client.SetSignType("RSA2")
	if err := client.SetCertSnByPath(cfg.AppCertPath, cfg.RootCertPath, cfg.AlipayCertPath); err != nil {
		return nil, fmt.Errorf("alipay: set cert sn: %w", err)
	}
	return client, nil
}

func validateChannelConfig(cfg ChannelConfig) error {
	if strings.TrimSpace(cfg.AppID) == "" {
		return errors.New("alipay: app id is required")
	}
	if strings.TrimSpace(cfg.PrivateKey) == "" {
		return errors.New("alipay: private key is required")
	}
	if strings.TrimSpace(cfg.AppCertPath) == "" {
		return errors.New("alipay: app cert path is required")
	}
	if strings.TrimSpace(cfg.AlipayCertPath) == "" {
		return errors.New("alipay: alipay cert path is required")
	}
	if strings.TrimSpace(cfg.RootCertPath) == "" {
		return errors.New("alipay: root cert path is required")
	}
	if strings.TrimSpace(cfg.NotifyURL) == "" {
		return errors.New("alipay: notify url is required")
	}
	return nil
}
