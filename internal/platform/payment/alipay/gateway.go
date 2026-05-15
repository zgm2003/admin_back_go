package alipay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-pay/gopay"
	gopayalipay "github.com/go-pay/gopay/alipay"
)

const maxAlipayBillBytes = 50 << 20

type GopayGateway struct {
	timeout time.Duration
}

func NewGopayGateway(timeout time.Duration) *GopayGateway {
	return &GopayGateway{timeout: timeout}
}

func (g *GopayGateway) Create(ctx context.Context, cfg ChannelConfig, req CreateRequest) (*CreateResponse, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := withGatewayTimeout(ctx, g.timeout)
	defer cancel()

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

func (g *GopayGateway) Query(ctx context.Context, cfg ChannelConfig, req QueryRequest) (*QueryResult, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := withGatewayTimeout(ctx, g.timeout)
	defer cancel()

	bm := gopay.BodyMap{}
	if strings.TrimSpace(req.OutTradeNo) != "" {
		bm.Set("out_trade_no", strings.TrimSpace(req.OutTradeNo))
	}
	if strings.TrimSpace(req.TradeNo) != "" {
		bm.Set("trade_no", strings.TrimSpace(req.TradeNo))
	}
	resp, err := client.TradeQuery(ctx, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay: query trade: %w", err)
	}
	if resp == nil || resp.Response == nil {
		return nil, errors.New("alipay: empty query response")
	}
	cents, err := parseYuanToCents(resp.Response.TotalAmount)
	if err != nil {
		cents = 0
	}
	return &QueryResult{
		OutTradeNo:       resp.Response.OutTradeNo,
		TradeNo:          resp.Response.TradeNo,
		TradeStatus:      resp.Response.TradeStatus,
		TotalAmountCents: cents,
		AppID:            cfg.AppID,
		Raw:              structToMap(resp.Response),
	}, nil
}

func (g *GopayGateway) Close(ctx context.Context, cfg ChannelConfig, req CloseRequest) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := withGatewayTimeout(ctx, g.timeout)
	defer cancel()

	bm := gopay.BodyMap{}
	if strings.TrimSpace(req.OutTradeNo) != "" {
		bm.Set("out_trade_no", strings.TrimSpace(req.OutTradeNo))
	}
	if strings.TrimSpace(req.TradeNo) != "" {
		bm.Set("trade_no", strings.TrimSpace(req.TradeNo))
	}
	if _, err := client.TradeClose(ctx, bm); err != nil {
		return fmt.Errorf("alipay: close trade: %w", err)
	}
	return nil
}

func (g *GopayGateway) TestConfig(ctx context.Context, cfg ChannelConfig) error {
	_ = ctx
	_, err := newClient(cfg)
	return err
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

func (g *GopayGateway) DownloadBill(ctx context.Context, cfg ChannelConfig, req BillDownloadRequest) (*BillDownloadResponse, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := withGatewayTimeout(ctx, g.timeout)
	defer cancel()

	billType := strings.TrimSpace(req.BillType)
	if billType == "" {
		billType = "trade"
	}
	billDate := strings.TrimSpace(req.BillDate)
	if billDate == "" {
		return nil, errors.New("alipay: bill date is required")
	}
	bm := gopay.BodyMap{}
	bm.Set("bill_type", billType)
	bm.Set("bill_date", billDate)
	resp, err := client.DataBillDownloadUrlQuery(ctx, bm)
	if err != nil {
		return nil, fmt.Errorf("alipay: query bill download url: %w", err)
	}
	if resp == nil || resp.Response == nil || strings.TrimSpace(resp.Response.BillDownloadUrl) == "" {
		return nil, errors.New("alipay: empty bill download url")
	}
	content, err := downloadBillContent(ctx, strings.TrimSpace(resp.Response.BillDownloadUrl))
	if err != nil {
		return nil, err
	}
	return &BillDownloadResponse{
		BillDownloadURL: strings.TrimSpace(resp.Response.BillDownloadUrl),
		Content:         content,
		Raw:             structToMap(resp.Response),
	}, nil
}

func (g *GopayGateway) SuccessBody() string {
	return "success"
}

func (g *GopayGateway) FailureBody() string {
	return "fail"
}

func downloadBillContent(ctx context.Context, billURL string) ([]byte, error) {
	return downloadBillContentWithClient(ctx, http.DefaultClient, billURL)
}

func downloadBillContentWithClient(ctx context.Context, httpClient *http.Client, billURL string) ([]byte, error) {
	if strings.TrimSpace(billURL) == "" {
		return nil, errors.New("alipay: bill download url is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, billURL, nil)
	if err != nil {
		return nil, fmt.Errorf("alipay: create bill download request: %w", err)
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("alipay: download bill: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("alipay: download bill http status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAlipayBillBytes+1))
	if err != nil {
		return nil, fmt.Errorf("alipay: read bill content: %w", err)
	}
	if len(data) > maxAlipayBillBytes {
		return nil, fmt.Errorf("alipay: bill content exceeds %d bytes", maxAlipayBillBytes)
	}
	if len(data) == 0 {
		return nil, errors.New("alipay: empty bill content")
	}
	return data, nil
}

func withGatewayTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
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

func structToMap(value any) map[string]any {
	buf, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return map[string]any{}
	}
	return out
}
