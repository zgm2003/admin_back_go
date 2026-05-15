package alipay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"github.com/go-pay/gopay"
	gopayalipay "github.com/go-pay/gopay/alipay"
)

const alipayTimeLayout = "2006-01-02 15:04:05"

type GopayGateway struct{}

func NewGopayGateway() *GopayGateway {
	return &GopayGateway{}
}

func (g *GopayGateway) TestConfig(ctx context.Context, cfg ChannelConfig) error {
	_ = ctx
	_, err := newClient(cfg)
	return err
}

func (g *GopayGateway) Pay(ctx context.Context, cfg ChannelConfig, in PayInput) (*PayResult, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	body, err := buildPayBody(in)
	if err != nil {
		return nil, err
	}
	var payURL string
	switch strings.TrimSpace(in.Method) {
	case enum.PaymentMethodWeb:
		payURL, err = client.TradePagePay(ctx, body)
	case enum.PaymentMethodH5:
		payURL, err = client.TradeWapPay(ctx, body)
	default:
		return nil, fmt.Errorf("alipay: unsupported pay method %q", in.Method)
	}
	if err != nil {
		return nil, fmt.Errorf("alipay: create pay url: %w", err)
	}
	return &PayResult{PayURL: payURL}, nil
}

func (g *GopayGateway) Query(ctx context.Context, cfg ChannelConfig, outTradeNo string) (*QueryResult, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	rsp, err := client.TradeQuery(ctx, buildOutTradeNoBody(outTradeNo))
	if err != nil {
		return nil, fmt.Errorf("alipay: query: %w", err)
	}
	if rsp == nil || rsp.Response == nil {
		return nil, errors.New("alipay: query empty response")
	}
	paidAt, err := parseAlipayTime(rsp.Response.SendPayDate)
	if err != nil {
		return nil, err
	}
	return &QueryResult{
		TradeNo: rsp.Response.TradeNo,
		Status:  rsp.Response.TradeStatus,
		PaidAt:  paidAt,
	}, nil
}

func (g *GopayGateway) Close(ctx context.Context, cfg ChannelConfig, outTradeNo string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	rsp, err := client.TradeClose(ctx, buildOutTradeNoBody(outTradeNo))
	if err != nil {
		return fmt.Errorf("alipay: close: %w", err)
	}
	if rsp == nil || rsp.Response == nil {
		return errors.New("alipay: close empty response")
	}
	return nil
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

func formatAmountCents(cents int64) (string, error) {
	if cents <= 0 {
		return "", errors.New("alipay: amount cents must be positive")
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100), nil
}

func buildPayBody(input PayInput) (gopay.BodyMap, error) {
	if strings.TrimSpace(input.OutTradeNo) == "" {
		return nil, errors.New("alipay: out trade no is required")
	}
	if strings.TrimSpace(input.Subject) == "" {
		return nil, errors.New("alipay: subject is required")
	}
	amount, err := formatAmountCents(input.AmountCents)
	if err != nil {
		return nil, err
	}
	body := gopay.BodyMap{}
	body.Set("out_trade_no", strings.TrimSpace(input.OutTradeNo))
	body.Set("subject", strings.TrimSpace(input.Subject))
	body.Set("total_amount", amount)
	if strings.TrimSpace(input.ReturnURL) != "" {
		body.Set("return_url", strings.TrimSpace(input.ReturnURL))
	}
	if !input.ExpiredAt.IsZero() {
		body.Set("time_expire", input.ExpiredAt.Format(alipayTimeLayout))
	}
	return body, nil
}

func buildOutTradeNoBody(outTradeNo string) gopay.BodyMap {
	body := gopay.BodyMap{}
	if strings.TrimSpace(outTradeNo) != "" {
		body.Set("out_trade_no", strings.TrimSpace(outTradeNo))
	}
	return body
}

func parseAlipayTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation(alipayTimeLayout, value, time.Local)
	if err != nil {
		return nil, fmt.Errorf("alipay: parse paid time: %w", err)
	}
	return &parsed, nil
}
