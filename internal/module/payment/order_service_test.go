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

func TestCreateOrderStoresPendingOrder(t *testing.T) {
	repo := newFakeOrderRepo()
	repo.config = enabledOrderConfig()
	service := newOrderService(repo, &fakeOrderGateway{})

	result, appErr := service.CreateOrder(context.Background(), OrderCreateInput{
		ConfigCode:    repo.config.Code,
		PayMethod:     enum.PaymentMethodWeb,
		Subject:       "测试订单",
		AmountCents:   100,
		ReturnURL:     "https://example.test/return",
		ExpireMinutes: 15,
	})
	if appErr != nil {
		t.Fatalf("CreateOrder error=%v", appErr)
	}
	if result.Status != orderStatusPending || repo.order == nil {
		t.Fatalf("unexpected create result=%#v order=%#v", result, repo.order)
	}
	if repo.order.ConfigID != repo.config.ID || repo.order.ConfigCode != repo.config.Code || repo.order.Provider != providerAlipay {
		t.Fatalf("config snapshot mismatch: %#v", repo.order)
	}
	if repo.order.Status != orderStatusPending || repo.order.ExpiredAt.Sub(fixedOrderNow()) != 15*time.Minute {
		t.Fatalf("order state mismatch: %#v", repo.order)
	}
}

func TestCreateOrderRejectsDisabledConfig(t *testing.T) {
	repo := newFakeOrderRepo()
	repo.config = enabledOrderConfig()
	repo.config.Status = enum.CommonNo
	service := newOrderService(repo, &fakeOrderGateway{})

	_, appErr := service.CreateOrder(context.Background(), validOrderCreateInput())
	if appErr == nil || !strings.Contains(appErr.Message, "未启用") {
		t.Fatalf("expected disabled config error, got %v", appErr)
	}
}

func TestCreateOrderRejectsDisabledPayMethod(t *testing.T) {
	repo := newFakeOrderRepo()
	repo.config = enabledOrderConfig()
	repo.config.EnabledMethodsJSON = mustConfigJSON([]string{enum.PaymentMethodWeb})
	service := newOrderService(repo, &fakeOrderGateway{})

	input := validOrderCreateInput()
	input.PayMethod = enum.PaymentMethodH5
	_, appErr := service.CreateOrder(context.Background(), input)
	if appErr == nil || !strings.Contains(appErr.Message, "支付方式") {
		t.Fatalf("expected method error, got %v", appErr)
	}
}

func TestPayOrderChangesPendingToPaying(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPending)
	gw := &fakeOrderGateway{payResult: &gateway.PayResult{PayURL: "https://pay.example.test"}}
	service := newOrderService(repo, gw)

	result, appErr := service.PayOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("PayOrder error=%v", appErr)
	}
	if result.Status != orderStatusPaying || repo.order.Status != orderStatusPaying || repo.order.PayURL == "" {
		t.Fatalf("unexpected pay state result=%#v order=%#v", result, repo.order)
	}
	if gw.payInput.OutTradeNo != repo.order.OrderNo || gw.payInput.ReturnURL != repo.order.ReturnURL {
		t.Fatalf("unexpected gateway input: %#v", gw.payInput)
	}
}

func TestPayOrderStoresFailureReasonWhenGatewayFails(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPending)
	gw := &fakeOrderGateway{payErr: errors.New("gateway down")}
	service := newOrderService(repo, gw)

	_, appErr := service.PayOrder(context.Background(), repo.order.ID)
	if appErr == nil {
		t.Fatal("expected pay error")
	}
	if repo.order.Status != orderStatusFailed || !strings.Contains(repo.order.FailureReason, "gateway down") {
		t.Fatalf("expected failed order, got %#v", repo.order)
	}
}

func TestPayOrderReturnsExistingPayURLForPayingOrder(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPaying)
	repo.order.PayURL = "https://pay.example.test/existing"
	gw := &fakeOrderGateway{}
	service := newOrderService(repo, gw)

	result, appErr := service.PayOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("PayOrder error=%v", appErr)
	}
	if result.PayURL != repo.order.PayURL || gw.payCalled {
		t.Fatalf("expected idempotent existing pay url, result=%#v gateway=%#v", result, gw)
	}
}

func TestPayOrderAllowsFailedRetry(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusFailed)
	repo.order.FailureReason = "previous error"
	gw := &fakeOrderGateway{payResult: &gateway.PayResult{PayURL: "https://pay.example.test/retry"}}
	service := newOrderService(repo, gw)

	result, appErr := service.PayOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("PayOrder error=%v", appErr)
	}
	if result.Status != orderStatusPaying || repo.order.PayURL == "" || repo.order.FailureReason != "" {
		t.Fatalf("expected failed order retry to paying, result=%#v order=%#v", result, repo.order)
	}
}

func TestSyncOrderMapsTradeSuccessToPaid(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPaying)
	paidAt := fixedOrderNow().Add(2 * time.Minute)
	gw := &fakeOrderGateway{queryResult: &gateway.QueryResult{Status: "TRADE_SUCCESS", TradeNo: "202605152200", PaidAt: &paidAt}}
	service := newOrderService(repo, gw)

	result, appErr := service.SyncOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("SyncOrder error=%v", appErr)
	}
	if result.Status != orderStatusPaid || repo.order.AlipayTradeNo != "202605152200" || repo.order.PaidAt == nil {
		t.Fatalf("expected paid order, result=%#v order=%#v", result, repo.order)
	}
}

func TestSyncOrderMapsTradeClosedToClosed(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPaying)
	gw := &fakeOrderGateway{queryResult: &gateway.QueryResult{Status: "TRADE_CLOSED"}}
	service := newOrderService(repo, gw)

	result, appErr := service.SyncOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("SyncOrder error=%v", appErr)
	}
	if result.Status != orderStatusClosed || repo.order.ClosedAt == nil {
		t.Fatalf("expected closed order, result=%#v order=%#v", result, repo.order)
	}
}

func TestCloseOrderRejectsPaid(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPaid)
	service := newOrderService(repo, &fakeOrderGateway{})

	_, appErr := service.CloseOrder(context.Background(), repo.order.ID)
	if appErr == nil || !strings.Contains(appErr.Message, "已支付") {
		t.Fatalf("expected paid close rejection, got %v", appErr)
	}
}

func TestCloseOrderClosesPendingLocally(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPending)
	gw := &fakeOrderGateway{}
	service := newOrderService(repo, gw)

	result, appErr := service.CloseOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("CloseOrder error=%v", appErr)
	}
	if result.Status != orderStatusClosed || repo.order.ClosedAt == nil || gw.closeCount != 0 {
		t.Fatalf("expected local close, result=%#v order=%#v gateway=%#v", result, repo.order, gw)
	}
}

func TestCloseOrderClosesFailedLocally(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusFailed)
	gw := &fakeOrderGateway{}
	service := newOrderService(repo, gw)

	result, appErr := service.CloseOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("CloseOrder error=%v", appErr)
	}
	if result.Status != orderStatusClosed || repo.order.ClosedAt == nil || gw.closeCount != 0 {
		t.Fatalf("expected failed local close, result=%#v order=%#v gateway=%#v", result, repo.order, gw)
	}
}

func TestOrderListItemIncludesPayURLForPayingReopen(t *testing.T) {
	row := *newFakeOrderRepoWithOrder(orderStatusPaying).order
	row.PayURL = "https://pay.example.test/existing"

	item := orderListItem(row)

	if item.PayURL != row.PayURL {
		t.Fatalf("expected list item pay_url for reopen action, got %#v", item)
	}
}

func TestCloseOrderCallsGatewayForPaying(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPaying)
	gw := &fakeOrderGateway{}
	service := newOrderService(repo, gw)

	result, appErr := service.CloseOrder(context.Background(), repo.order.ID)
	if appErr != nil {
		t.Fatalf("CloseOrder error=%v", appErr)
	}
	if result.Status != orderStatusClosed || gw.closeCount != 1 || gw.closeOutTradeNo != repo.order.OrderNo {
		t.Fatalf("expected gateway close, result=%#v gateway=%#v", result, gw)
	}
}

func validOrderCreateInput() OrderCreateInput {
	return OrderCreateInput{
		ConfigCode:    "alipay_default",
		PayMethod:     enum.PaymentMethodWeb,
		Subject:       "测试订单",
		AmountCents:   100,
		ReturnURL:     "https://example.test/return",
		ExpireMinutes: 30,
	}
}

func enabledOrderConfig() *Config {
	return &Config{
		ID:                 1,
		Provider:           providerAlipay,
		Code:               "alipay_default",
		Name:               "支付宝默认配置",
		AppID:              "2026000000000000",
		PrivateKeyEnc:      "enc:PRIVATE_KEY",
		AppCertPath:        "runtime/app.crt",
		PlatformCertPath:   "runtime/alipay.crt",
		RootCertPath:       "runtime/root.crt",
		NotifyURL:          "https://example.test/notify",
		Environment:        environmentSandbox,
		EnabledMethodsJSON: mustConfigJSON([]string{enum.PaymentMethodWeb, enum.PaymentMethodH5}),
		Status:             enum.CommonYes,
		IsDel:              enum.CommonNo,
	}
}

func newFakeOrderRepoWithOrder(status string) *fakeOrderRepo {
	repo := newFakeOrderRepo()
	repo.config = enabledOrderConfig()
	repo.order = &Order{
		ID:          1,
		OrderNo:     "PAY20260515100000000000",
		ConfigID:    repo.config.ID,
		ConfigCode:  repo.config.Code,
		Provider:    repo.config.Provider,
		PayMethod:   enum.PaymentMethodWeb,
		Subject:     "测试订单",
		AmountCents: 100,
		Status:      status,
		ReturnURL:   "https://example.test/return",
		ExpiredAt:   fixedOrderNow().Add(time.Hour),
		IsDel:       enum.CommonNo,
		CreatedAt:   fixedOrderNow(),
		UpdatedAt:   fixedOrderNow(),
	}
	return repo
}

func newOrderService(repo *fakeOrderRepo, gw *fakeOrderGateway) *Service {
	return NewService(Dependencies{
		Repository:   repo,
		Gateway:      gw,
		Secretbox:    &fakeSecretbox{},
		CertResolver: fakeResolver{},
		CertStore:    &fakeCertStore{},
		Now:          fixedOrderNow,
	})
}

func fixedOrderNow() time.Time { return time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC) }

type fakeOrderRepo struct {
	config *Config
	order  *Order
}

func newFakeOrderRepo() *fakeOrderRepo { return &fakeOrderRepo{} }

func (r *fakeOrderRepo) ListConfigs(ctx context.Context, query ConfigListQuery) ([]Config, int64, error) {
	if r.config == nil {
		return nil, 0, nil
	}
	return []Config{*r.config}, 1, nil
}
func (r *fakeOrderRepo) GetConfig(ctx context.Context, id int64) (*Config, error) {
	if r.config == nil || r.config.ID != id {
		return nil, nil
	}
	copy := *r.config
	return &copy, nil
}
func (r *fakeOrderRepo) GetConfigByCode(ctx context.Context, code string) (*Config, error) {
	if r.config == nil || r.config.Code != strings.TrimSpace(code) {
		return nil, nil
	}
	copy := *r.config
	return &copy, nil
}
func (r *fakeOrderRepo) CreateConfig(ctx context.Context, cfg Config) (int64, error) { return 0, nil }
func (r *fakeOrderRepo) UpdateConfig(ctx context.Context, cfg Config, keepPrivateKey bool) error {
	return nil
}
func (r *fakeOrderRepo) ChangeConfigStatus(ctx context.Context, id int64, status int) error {
	return nil
}
func (r *fakeOrderRepo) DeleteConfig(ctx context.Context, id int64) error { return nil }
func (r *fakeOrderRepo) ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error) {
	if r.order == nil {
		return nil, 0, nil
	}
	return []Order{*r.order}, 1, nil
}
func (r *fakeOrderRepo) GetOrder(ctx context.Context, id int64) (*Order, error) {
	if r.order == nil || r.order.ID != id {
		return nil, nil
	}
	copy := *r.order
	return &copy, nil
}
func (r *fakeOrderRepo) CreateOrder(ctx context.Context, order Order) (int64, error) {
	order.ID = 1
	r.order = &order
	return order.ID, nil
}
func (r *fakeOrderRepo) UpdateOrderPaying(ctx context.Context, id int64, payURL string) error {
	r.order.Status = orderStatusPaying
	r.order.PayURL = payURL
	r.order.FailureReason = ""
	return nil
}
func (r *fakeOrderRepo) UpdateOrderFailed(ctx context.Context, id int64, reason string) error {
	r.order.Status = orderStatusFailed
	r.order.FailureReason = reason
	return nil
}
func (r *fakeOrderRepo) UpdateOrderPaid(ctx context.Context, id int64, tradeNo string, paidAt time.Time) error {
	r.order.Status = orderStatusPaid
	r.order.AlipayTradeNo = tradeNo
	r.order.PaidAt = &paidAt
	r.order.FailureReason = ""
	return nil
}
func (r *fakeOrderRepo) UpdateOrderClosed(ctx context.Context, id int64, closedAt time.Time) error {
	r.order.Status = orderStatusClosed
	r.order.ClosedAt = &closedAt
	return nil
}
func (r *fakeOrderRepo) ListEnabledOrderConfigOptions(ctx context.Context) ([]Config, error) {
	if r.config == nil || r.config.Status != enum.CommonYes {
		return nil, nil
	}
	return []Config{*r.config}, nil
}

type fakeOrderGateway struct {
	payResult       *gateway.PayResult
	payErr          error
	queryResult     *gateway.QueryResult
	queryErr        error
	closeErr        error
	closeCount      int
	payCalled       bool
	payInput        gateway.PayInput
	closeOutTradeNo string
}

func (g *fakeOrderGateway) TestConfig(ctx context.Context, cfg gateway.ChannelConfig) error {
	return nil
}
func (g *fakeOrderGateway) Pay(ctx context.Context, cfg gateway.ChannelConfig, in gateway.PayInput) (*gateway.PayResult, error) {
	g.payCalled = true
	g.payInput = in
	if g.payErr != nil {
		return nil, g.payErr
	}
	if g.payResult == nil {
		return &gateway.PayResult{PayURL: "https://pay.example.test"}, nil
	}
	return g.payResult, nil
}
func (g *fakeOrderGateway) Query(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) (*gateway.QueryResult, error) {
	if g.queryErr != nil {
		return nil, g.queryErr
	}
	if g.queryResult == nil {
		return &gateway.QueryResult{Status: "WAIT_BUYER_PAY"}, nil
	}
	return g.queryResult, nil
}
func (g *fakeOrderGateway) Close(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) error {
	g.closeCount++
	g.closeOutTradeNo = outTradeNo
	return g.closeErr
}
