package alipay

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
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

func (g *GopayGateway) VerifyNotify(ctx context.Context, cfg ChannelConfig, form url.Values) (*NotifyPayload, error) {
	_ = ctx
	if err := validateChannelConfig(cfg); err != nil {
		return nil, err
	}
	payload, err := ParseNotifyPayload(form)
	if err != nil {
		return nil, err
	}
	body, err := gopayalipay.ParseNotifyByURLValues(form)
	if err != nil {
		return nil, fmt.Errorf("alipay: parse notify: %w", err)
	}
	ok, err := gopayalipay.VerifySignWithCert(strings.TrimSpace(cfg.AlipayCertPath), body)
	if err != nil {
		return nil, fmt.Errorf("alipay: verify notify sign: %w", err)
	}
	if !ok {
		return nil, errors.New("alipay: verify notify sign failed")
	}
	return payload, nil
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

func ParseNotifyPayload(form url.Values) (*NotifyPayload, error) {
	amountCents, err := parseAmountCents(form.Get("total_amount"))
	if err != nil {
		return nil, err
	}
	raw := make(map[string]string, len(form))
	for key, values := range form {
		if len(values) == 0 {
			continue
		}
		raw[key] = values[0]
	}
	return &NotifyPayload{
		NotifyID:         strings.TrimSpace(form.Get("notify_id")),
		OutTradeNo:       strings.TrimSpace(form.Get("out_trade_no")),
		TradeNo:          strings.TrimSpace(form.Get("trade_no")),
		TradeStatus:      strings.TrimSpace(form.Get("trade_status")),
		AppID:            strings.TrimSpace(form.Get("app_id")),
		TotalAmountCents: amountCents,
		Raw:              raw,
	}, nil
}

func parseAmountCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("alipay: total amount is required")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("alipay: invalid total amount %q", value)
	}
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || yuan < 0 {
		return 0, fmt.Errorf("alipay: invalid total amount %q", value)
	}
	centText := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("alipay: invalid total amount %q", value)
		}
		centText = (parts[1] + "00")[:2]
	}
	cents, err := strconv.ParseInt(centText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("alipay: invalid total amount %q", value)
	}
	return yuan*100 + cents, nil
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
