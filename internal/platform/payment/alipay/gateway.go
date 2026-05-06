package alipay

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-pay/gopay"
	gopayalipay "github.com/go-pay/gopay/alipay"
)

type GopayGateway struct{}

func NewGopayGateway() *GopayGateway {
	return &GopayGateway{}
}

func (g *GopayGateway) Create(ctx context.Context, cfg ChannelConfig, req CreateRequest) (*CreateResponse, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	bm := gopay.BodyMap{}
	bm.Set("subject", strings.TrimSpace(req.Subject))
	bm.Set("out_trade_no", strings.TrimSpace(req.OutTradeNo))
	bm.Set("total_amount", formatCents(req.AmountCents))
	if strings.TrimSpace(req.ReturnURL) != "" {
		client.SetReturnUrl(strings.TrimSpace(req.ReturnURL))
	}

	var content string
	switch strings.TrimSpace(req.PayMethod) {
	case "web":
		content, err = client.TradePagePay(ctx, bm)
	case "h5":
		content, err = client.TradeWapPay(ctx, bm)
	default:
		return nil, fmt.Errorf("alipay: unsupported pay method %s", req.PayMethod)
	}
	if err != nil {
		return nil, fmt.Errorf("alipay: create pay: %w", err)
	}
	return &CreateResponse{
		Mode:    "external",
		Content: content,
		Raw:     map[string]any{"content": content},
	}, nil
}

func (g *GopayGateway) VerifyNotify(ctx context.Context, cfg ChannelConfig, req NotifyRequest) (*NotifyResult, error) {
	_ = ctx
	if err := validateChannelConfig(cfg); err != nil {
		return nil, err
	}
	if len(req.Form) == 0 {
		return nil, errors.New("alipay: empty notify form")
	}

	bm := gopay.BodyMap{}
	for k, v := range req.Form {
		bm.Set(k, v)
	}
	ok, err := gopayalipay.VerifySignWithCert(cfg.AlipayCertPath, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay: verify notify: %w", err)
	}
	if !ok {
		return nil, errors.New("alipay: invalid notify signature")
	}
	cents, err := parseYuanToCents(bm.GetString("total_amount"))
	if err != nil {
		return nil, err
	}
	return &NotifyResult{
		OutTradeNo:       bm.GetString("out_trade_no"),
		TradeNo:          bm.GetString("trade_no"),
		TradeStatus:      bm.GetString("trade_status"),
		TotalAmountCents: cents,
		AppID:            bm.GetString("app_id"),
		Raw:              bodyMapToMap(bm),
	}, nil
}

func (g *GopayGateway) SuccessBody() string {
	return "success"
}

func (g *GopayGateway) FailureBody() string {
	return "fail"
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

func formatCents(cents int) string {
	yuan := cents / 100
	fen := cents % 100
	return fmt.Sprintf("%d.%02d", yuan, fen)
}

func parseYuanToCents(amount string) (int, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return 0, errors.New("alipay: amount is required")
	}
	if strings.HasPrefix(amount, "-") {
		return 0, fmt.Errorf("alipay: invalid amount %s", amount)
	}
	parts := strings.Split(amount, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("alipay: invalid amount %s", amount)
	}
	yuan, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("alipay: invalid amount %s", amount)
	}
	cents := 0
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("alipay: invalid amount precision %s", amount)
		}
		fenText := parts[1]
		if len(fenText) == 1 {
			fenText += "0"
		}
		cents, err = strconv.Atoi(fenText)
		if err != nil {
			return 0, fmt.Errorf("alipay: invalid amount %s", amount)
		}
	}
	return yuan*100 + cents, nil
}

func bodyMapToMap(bm gopay.BodyMap) map[string]any {
	raw := make(map[string]any, len(bm))
	bm.Range(func(k string, v any) bool {
		raw[k] = v
		return true
	})
	return raw
}
