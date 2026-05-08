package payment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	gateway "admin_back_go/internal/platform/payment"
)

func TestCreateOrderDoesNotTouchWallet(t *testing.T) {
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	service := NewService(Dependencies{
		Repository:      repo,
		Gateway:         &fakeGateway{},
		NumberGenerator: fixedNumberGenerator("P260508123456000007"),
		Now:             fixedNow,
	})

	resp, appErr := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID: 9, ChannelID: 1, PayMethod: enum.PaymentMethodWeb, Subject: "充值", AmountCents: 1000,
	})
	if appErr != nil {
		t.Fatalf("CreateOrder returned app error: %v", appErr)
	}
	if resp.OrderNo != "P260508123456000007" {
		t.Fatalf("unexpected order no: %s", resp.OrderNo)
	}
	if repo.createdOrder == nil {
		t.Fatalf("order was not created")
	}
	if repo.createdOrder.OutTradeNo != nil {
		t.Fatalf("CreateOrder must keep out_trade_no nil before payment attempt")
	}
	if repo.walletTouched {
		t.Fatalf("CreateOrder touched wallet path")
	}
}

func TestPayOrderMarksPaying(t *testing.T) {
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{
		ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay,
		PayMethod: enum.PaymentMethodWeb, Subject: "充值", AmountCents: 1000, Status: enum.PaymentOrderPending,
		ExpiredAt: fixedNow().Add(30 * time.Minute),
	}
	service := NewService(Dependencies{
		Repository:      repo,
		Gateway:         &fakeGateway{createResult: &gateway.CreatePayResult{Mode: "url", Content: "https://pay.example/P1", Raw: map[string]any{"ok": true}}},
		NumberGenerator: fixedNumberGenerator("unused"),
		Now:             fixedNow,
	})

	resp, appErr := service.PayOrder(context.Background(), 9, "P1", "https://return.example")
	if appErr != nil {
		t.Fatalf("PayOrder returned app error: %v", appErr)
	}
	if resp.OutTradeNo != "P1" {
		t.Fatalf("unexpected out_trade_no: %s", resp.OutTradeNo)
	}
	if repo.markPaying.OutTradeNo != "P1" || repo.markPaying.PayURL != "https://pay.example/P1" {
		t.Fatalf("MarkOrderPaying not called with gateway data: %+v", repo.markPaying)
	}
	if len(repo.events) != 1 || repo.events[0].ProcessStatus != enum.PaymentEventSuccess {
		t.Fatalf("expected success create event, got %+v", repo.events)
	}
}

func TestPayOrderGatewayFailureWritesFailedEvent(t *testing.T) {
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, Subject: "充值", AmountCents: 1000, Status: enum.PaymentOrderPending, ExpiredAt: fixedNow().Add(time.Minute)}
	service := NewService(Dependencies{
		Repository:      repo,
		Gateway:         &fakeGateway{createErr: errors.New("gateway down")},
		NumberGenerator: fixedNumberGenerator("P260508123456000009"),
		Now:             fixedNow,
	})

	_, appErr := service.PayOrder(context.Background(), 9, "P1", "")
	if appErr == nil {
		t.Fatalf("expected app error")
	}
	if len(repo.events) != 1 || repo.events[0].ProcessStatus != enum.PaymentEventFailed {
		t.Fatalf("expected failed create event, got %+v", repo.events)
	}
}

func TestPayOrderGatewayFailureEventErrorReturnsAuditError(t *testing.T) {
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, Subject: "充值", AmountCents: 1000, Status: enum.PaymentOrderPending, ExpiredAt: fixedNow().Add(time.Minute)}
	repo.createEventErr = errors.New("event insert failed")
	service := NewService(Dependencies{
		Repository: repo,
		Gateway:    &fakeGateway{createErr: errors.New("gateway down")},
		Now:        fixedNow,
	})

	_, appErr := service.PayOrder(context.Background(), 9, "P1", "")
	if appErr == nil || !strings.Contains(appErr.Message, "记录支付事件失败") || !errors.Is(appErr.Cause, repo.createEventErr) {
		t.Fatalf("expected audit event error, got %v cause=%v", appErr, appErr.Unwrap())
	}
}

func TestPayOrderSuccessEventErrorReturnsAfterMarkPaying(t *testing.T) {
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, Subject: "充值", AmountCents: 1000, Status: enum.PaymentOrderPending, ExpiredAt: fixedNow().Add(time.Minute)}
	repo.createEventErr = errors.New("event insert failed")
	service := NewService(Dependencies{
		Repository: repo,
		Gateway:    &fakeGateway{createResult: &gateway.CreatePayResult{Mode: "url", Content: "https://pay.example/P1"}},
		Now:        fixedNow,
	})

	_, appErr := service.PayOrder(context.Background(), 9, "P1", "")
	if appErr == nil || !strings.Contains(appErr.Message, "记录支付事件失败") || repo.markPaying.OutTradeNo != "P1" {
		t.Fatalf("expected event error after mark paying, appErr=%v mark=%+v", appErr, repo.markPaying)
	}
}

func TestOrderDTOMapsNilOutTradeNoToEmptyString(t *testing.T) {
	item := orderListItem(Order{OrderNo: "P1", Status: enum.PaymentOrderPending, CreatedAt: fixedNow(), ExpiredAt: fixedNow()})
	if item.OutTradeNo != "" {
		t.Fatalf("nil out_trade_no should map to empty string, got %q", item.OutTradeNo)
	}
}

func TestHandleAlipayNotifySuccessAndDuplicateIdempotent(t *testing.T) {
	repo := newFakePaymentRepo()
	outTradeNo := "P1"
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, OutTradeNo: &outTradeNo, AmountCents: 1000, Status: enum.PaymentOrderPaying}
	service := NewService(Dependencies{
		Repository: repo,
		Gateway: &fakeGateway{notifyResult: &gateway.NotifyResult{
			OutTradeNo: outTradeNo, TradeNo: "trade-1", TradeStatus: "TRADE_SUCCESS", AmountCents: 1000, AppID: "app-1",
			Raw: map[string]any{"sign": "secret", "trade_no": "trade-1"},
		}},
		Now: fixedNow,
	})

	body, appErr := service.HandleAlipayNotify(context.Background(), NotifyInput{Form: map[string]string{"out_trade_no": outTradeNo, "private_key": "must-hide"}})
	if appErr != nil || body != "success" {
		t.Fatalf("first notify body=%q err=%v", body, appErr)
	}
	body, appErr = service.HandleAlipayNotify(context.Background(), NotifyInput{Form: map[string]string{"out_trade_no": outTradeNo}})
	if appErr != nil || body != "success" {
		t.Fatalf("duplicate notify body=%q err=%v", body, appErr)
	}
	if repo.succeededCount != 1 {
		t.Fatalf("notify should update success once, got %d", repo.succeededCount)
	}
	if !repo.lockInTx || !repo.successInTx || len(repo.eventInTx) != 2 {
		t.Fatalf("notify did not lock/update/event inside tx: lock=%v success=%v events=%v", repo.lockInTx, repo.successInTx, repo.eventInTx)
	}
	if repo.eventInTx[0] != enum.PaymentEventSuccess || repo.eventInTx[1] != enum.PaymentEventIgnored {
		t.Fatalf("expected success then ignored events, got %v", repo.eventInTx)
	}
	if repo.listOrders != 0 {
		t.Fatalf("notify must not scan orders, ListOrders called %d times", repo.listOrders)
	}
	if len(repo.events) == 0 || containsSensitive(repo.events[0].RequestData) || containsSensitive(repo.events[0].ResponseData) {
		t.Fatalf("event leaked sensitive data: %+v", repo.events[0])
	}
}

func TestHandleAlipayNotifyMismatchDoesNotChangeOrder(t *testing.T) {
	repo := newFakePaymentRepo()
	outTradeNo := "P1"
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, OutTradeNo: &outTradeNo, AmountCents: 1000, Status: enum.PaymentOrderPaying}
	service := NewService(Dependencies{
		Repository: repo,
		Gateway: &fakeGateway{notifyResult: &gateway.NotifyResult{
			OutTradeNo: outTradeNo, TradeNo: "trade-1", TradeStatus: "TRADE_SUCCESS", AmountCents: 999, AppID: "app-1",
		}},
		Now: fixedNow,
	})

	body, appErr := service.HandleAlipayNotify(context.Background(), NotifyInput{Form: map[string]string{"out_trade_no": outTradeNo}})
	if appErr != nil || body != "fail" {
		t.Fatalf("mismatch notify body=%q err=%v", body, appErr)
	}
	if repo.succeededCount != 0 {
		t.Fatalf("mismatch notify changed order")
	}
}

func TestHandleAlipayNotifyMissingOrNotFoundReturnsFailWithoutAppError(t *testing.T) {
	repo := newFakePaymentRepo()
	service := NewService(Dependencies{Repository: repo, Gateway: &fakeGateway{}, Now: fixedNow})

	body, appErr := service.HandleAlipayNotify(context.Background(), NotifyInput{Form: map[string]string{}})
	if appErr != nil || body != "fail" {
		t.Fatalf("missing out_trade_no body=%q err=%v", body, appErr)
	}
	body, appErr = service.HandleAlipayNotify(context.Background(), NotifyInput{Form: map[string]string{"out_trade_no": "P404"}})
	if appErr != nil || body != "fail" {
		t.Fatalf("not found body=%q err=%v", body, appErr)
	}
}

func TestGatewayConfigResolvesCertificatePaths(t *testing.T) {
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1", AppCertPath: "app.crt", AlipayCertPath: "alipay.crt", AlipayRootCertPath: "root.crt"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, Subject: "充值", AmountCents: 1000, Status: enum.PaymentOrderPending, ExpiredAt: fixedNow().Add(time.Minute)}
	gw := &fakeGateway{createResult: &gateway.CreatePayResult{Mode: "url", Content: "https://pay.example/P1"}}
	resolver := &fakeCertResolver{}
	service := NewService(Dependencies{Repository: repo, Gateway: gw, CertResolver: resolver, NumberGenerator: fixedNumberGenerator("unused"), Now: fixedNow})

	_, appErr := service.PayOrder(context.Background(), 9, "P1", "")
	if appErr != nil {
		t.Fatalf("PayOrder returned app error: %v", appErr)
	}
	if strings.Join(resolver.paths, ",") != "app.crt,alipay.crt,root.crt" {
		t.Fatalf("unexpected resolved paths: %v", resolver.paths)
	}
	if gw.lastCfg.AppCertPath != "resolved:app.crt" || gw.lastCfg.AlipayCertPath != "resolved:alipay.crt" || gw.lastCfg.RootCertPath != "resolved:root.crt" {
		t.Fatalf("gateway config did not use resolved paths: %+v", gw.lastCfg)
	}
}

func TestCloseExpiredOrdersClosesRemoteBeforeLocalClose(t *testing.T) {
	outTradeNo := "P1"
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, OutTradeNo: &outTradeNo, AmountCents: 1000, Status: enum.PaymentOrderPaying, ExpiredAt: fixedNow().Add(-time.Minute)}
	gw := &fakeGateway{queryResult: &gateway.QueryResult{OutTradeNo: outTradeNo, TradeStatus: "WAIT_BUYER_PAY", AmountCents: 1000, AppID: "app-1"}}
	service := NewService(Dependencies{Repository: repo, Gateway: gw, Now: fixedNow})

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredInput{Limit: 10, Now: fixedNow()})
	if err != nil {
		t.Fatalf("CloseExpiredOrders returned error: %v", err)
	}
	if gw.closeCalled != 1 || result.Closed != 1 || result.Deferred != 0 || repo.order.Status != enum.PaymentOrderClosed {
		t.Fatalf("unexpected close result closeCalled=%d result=%+v status=%d", gw.closeCalled, result, repo.order.Status)
	}
}

func TestCancelOrderRemoteCloseFailureDoesNotCloseLocalOrder(t *testing.T) {
	outTradeNo := "P1"
	repo := newFakePaymentRepo()
	repo.channel = &Channel{ID: 1, Provider: enum.PaymentProviderAlipay, Status: enum.CommonYes, SupportedMethods: enum.PaymentMethodWeb}
	repo.config = &ChannelConfig{ChannelID: 1, AppID: "app-1"}
	repo.order = &Order{ID: 1, OrderNo: "P1", UserID: 9, ChannelID: 1, Provider: enum.PaymentProviderAlipay, PayMethod: enum.PaymentMethodWeb, OutTradeNo: &outTradeNo, AmountCents: 1000, Status: enum.PaymentOrderPaying}
	gw := &fakeGateway{closeErr: errors.New("remote close failed")}
	service := NewService(Dependencies{Repository: repo, Gateway: gw, Now: fixedNow})

	appErr := service.CancelOrder(context.Background(), 9, "P1")
	if appErr == nil || !strings.Contains(appErr.Message, "关闭支付宝支付订单失败") {
		t.Fatalf("expected remote close app error, got %v", appErr)
	}
	if repo.order.Status != enum.PaymentOrderPaying || gw.closeCalled != 1 {
		t.Fatalf("local order should remain paying, status=%d closeCalled=%d", repo.order.Status, gw.closeCalled)
	}
}

func TestSanitizeSensitiveKeys(t *testing.T) {
	got := sanitizeMap(map[string]any{
		"privateKey":    "a",
		"appPrivateKey": "b",
		"token":         "c",
		"secret":        "d",
		"signature":     "e",
		"sign":          "f",
		"password":      "g",
		"normal":        "ok",
	})
	for _, key := range []string{"privateKey", "appPrivateKey", "token", "secret", "signature", "sign", "password"} {
		if got[key] != "***" {
			t.Fatalf("%s was not sanitized: %+v", key, got)
		}
	}
	if got["normal"] != "ok" {
		t.Fatalf("normal key should not be sanitized: %+v", got)
	}
}

type fixedNumberGenerator string

func (g fixedNumberGenerator) Next(ctx context.Context, prefix string) (string, error) {
	return string(g), nil
}

type fakeGateway struct {
	createResult *gateway.CreatePayResult
	createErr    error
	queryResult  *gateway.QueryResult
	queryErr     error
	notifyResult *gateway.NotifyResult
	notifyErr    error
	closeErr     error
	closeCalled  int
	lastCfg      gateway.ChannelConfig
}

func (g *fakeGateway) CreatePagePay(ctx context.Context, cfg gateway.ChannelConfig, req gateway.CreatePayRequest) (*gateway.CreatePayResult, error) {
	g.lastCfg = cfg
	if g.createErr != nil {
		return nil, g.createErr
	}
	if g.createResult != nil {
		return g.createResult, nil
	}
	return &gateway.CreatePayResult{Mode: "url", Content: "https://pay.example", Raw: map[string]any{}}, nil
}
func (g *fakeGateway) Query(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) (*gateway.QueryResult, error) {
	g.lastCfg = cfg
	return g.queryResult, g.queryErr
}
func (g *fakeGateway) VerifyNotify(ctx context.Context, cfg gateway.ChannelConfig, form map[string]string) (*gateway.NotifyResult, error) {
	g.lastCfg = cfg
	return g.notifyResult, g.notifyErr
}
func (g *fakeGateway) Close(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) error {
	g.lastCfg = cfg
	g.closeCalled++
	return g.closeErr
}
func (g *fakeGateway) SuccessBody() string { return "success" }
func (g *fakeGateway) FailureBody() string { return "fail" }

type fakeCertResolver struct {
	paths []string
	err   error
}

func (r *fakeCertResolver) Resolve(path string) (string, error) {
	r.paths = append(r.paths, path)
	if r.err != nil {
		return "", r.err
	}
	return "resolved:" + path, nil
}

type fakePaymentRepo struct {
	channel        *Channel
	config         *ChannelConfig
	order          *Order
	createdOrder   *Order
	events         []Event
	walletTouched  bool
	succeededCount int
	createEventErr error
	txDepth        int
	lockInTx       bool
	successInTx    bool
	eventInTx      []int
	listOrders     int
	markPaying     struct {
		OutTradeNo string
		PayURL     string
	}
}

func newFakePaymentRepo() *fakePaymentRepo { return &fakePaymentRepo{} }

func (r *fakePaymentRepo) WithTx(ctx context.Context, fn func(Repository) error) error {
	r.txDepth++
	defer func() { r.txDepth-- }()
	return fn(r)
}
func (r *fakePaymentRepo) ListChannels(ctx context.Context, query ChannelListQuery) ([]Channel, int64, error) {
	return nil, 0, nil
}
func (r *fakePaymentRepo) GetChannel(ctx context.Context, id int64) (*Channel, error) {
	return r.channel, nil
}
func (r *fakePaymentRepo) GetChannelConfig(ctx context.Context, channelID int64) (*ChannelConfig, error) {
	return r.config, nil
}
func (r *fakePaymentRepo) CreateChannel(ctx context.Context, channel Channel, cfg ChannelConfig) (int64, error) {
	return 0, nil
}
func (r *fakePaymentRepo) UpdateChannel(ctx context.Context, id int64, fields map[string]any, cfgFields map[string]any) error {
	return nil
}
func (r *fakePaymentRepo) ChangeChannelStatus(ctx context.Context, id int64, status int) error {
	return nil
}
func (r *fakePaymentRepo) DeleteChannel(ctx context.Context, id int64) error { return nil }
func (r *fakePaymentRepo) FindEnabledChannel(ctx context.Context, id int64) (*Channel, *ChannelConfig, error) {
	return r.channel, r.config, nil
}
func (r *fakePaymentRepo) CreateOrder(ctx context.Context, order Order) (*Order, error) {
	order.ID = 1
	r.createdOrder = &order
	r.order = &order
	return &order, nil
}
func (r *fakePaymentRepo) GetOrderByNo(ctx context.Context, orderNo string) (*Order, error) {
	return r.order, nil
}
func (r *fakePaymentRepo) GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, error) {
	if r.txDepth > 0 {
		r.lockInTx = true
	}
	return r.order, nil
}
func (r *fakePaymentRepo) GetOrderByID(ctx context.Context, id int64) (*Order, error) {
	return r.order, nil
}
func (r *fakePaymentRepo) ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error) {
	r.listOrders++
	if r.order == nil {
		return nil, 0, nil
	}
	return []Order{*r.order}, 1, nil
}
func (r *fakePaymentRepo) MarkOrderPaying(ctx context.Context, orderID int64, outTradeNo string, payURL string, returnURL string, now time.Time) error {
	r.markPaying.OutTradeNo = outTradeNo
	r.markPaying.PayURL = payURL
	if r.order != nil {
		r.order.OutTradeNo = &outTradeNo
		r.order.PayURL = payURL
		r.order.Status = enum.PaymentOrderPaying
	}
	return nil
}
func (r *fakePaymentRepo) MarkOrderSucceeded(ctx context.Context, orderID int64, tradeNo string, paidAt time.Time) error {
	if r.txDepth > 0 {
		r.successInTx = true
	}
	if r.order != nil && r.order.Status == enum.PaymentOrderSucceeded {
		return nil
	}
	r.succeededCount++
	if r.order != nil {
		r.order.Status = enum.PaymentOrderSucceeded
		r.order.TradeNo = tradeNo
		r.order.PaidAt = &paidAt
	}
	return nil
}
func (r *fakePaymentRepo) MarkOrderClosed(ctx context.Context, orderID int64, now time.Time) error {
	if r.order != nil {
		r.order.Status = enum.PaymentOrderClosed
	}
	return nil
}
func (r *fakePaymentRepo) CreateEvent(ctx context.Context, event Event) error {
	if r.createEventErr != nil {
		return r.createEventErr
	}
	if r.txDepth > 0 {
		r.eventInTx = append(r.eventInTx, event.ProcessStatus)
	}
	r.events = append(r.events, event)
	return nil
}
func (r *fakePaymentRepo) GetEventByID(ctx context.Context, id int64) (*Event, error) {
	if len(r.events) == 0 {
		return nil, nil
	}
	return &r.events[0], nil
}
func (r *fakePaymentRepo) ListEvents(ctx context.Context, query EventListQuery) ([]Event, int64, error) {
	return r.events, int64(len(r.events)), nil
}
func (r *fakePaymentRepo) ListExpiredOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	if r.order == nil {
		return nil, nil
	}
	return []Order{*r.order}, nil
}
func (r *fakePaymentRepo) ListPendingOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	if r.order == nil {
		return nil, nil
	}
	return []Order{*r.order}, nil
}

func containsSensitive(value string) bool {
	return strings.Contains(value, "must-hide") || strings.Contains(value, "secret")
}
