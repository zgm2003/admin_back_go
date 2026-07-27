package payment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	gateway "admin_back_go/internal/infra/payment"
	"admin_back_go/internal/shared/enum"
)

func TestRechargePageInitOmitsRecentAndDoesNotQueryRecentRecharges(t *testing.T) {
	repo := newFakeRechargeRepo()
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.RechargePageInit(context.Background(), 7)
	if appErr != nil {
		t.Fatalf("RechargePageInit error=%v", appErr)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal RechargePageInitResponse: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode RechargePageInitResponse: %v", err)
	}
	if _, exists := fields["recent"]; exists {
		t.Fatalf("RechargePageInitResponse must not publish recent: %s", payload)
	}
	if repo.listRechargeCalls != 0 {
		t.Fatalf("RechargePageInit must not query recharge history, calls=%d", repo.listRechargeCalls)
	}
}

func TestListRechargesStillReturnsPaginatedRecordsAndFilters(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.order = &Order{ID: 21, OrderNo: "PAY202607270001", Status: orderStatusPaying, PayURL: "https://pay.example.test"}
	repo.recharge = &Recharge{ID: 11, RechargeNo: "RCG202607270001", UserID: 7, PaymentOrderID: 21, PackageName: "50元", AmountCents: 5000, Status: rechargeStatusPaying, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.ListRecharges(context.Background(), RechargeListQuery{
		UserID: 7, CurrentPage: 2, PageSize: 10, Keyword: "  RCG2026  ", Status: "  paying  ", DateStart: "2026-07-01", DateEnd: "2026-07-31",
	})
	if appErr != nil {
		t.Fatalf("ListRecharges error=%v", appErr)
	}
	if len(result.List) != 1 || result.List[0].ID != 11 || result.List[0].PayURL != "https://pay.example.test" {
		t.Fatalf("ListRecharges records changed: %#v", result.List)
	}
	if result.Page.CurrentPage != 2 || result.Page.PageSize != 10 || result.Page.Total != 1 {
		t.Fatalf("ListRecharges page changed: %#v", result.Page)
	}
	if repo.listRechargeQuery.Keyword != "RCG2026" || repo.listRechargeQuery.Status != rechargeStatusPaying || repo.listRechargeQuery.DateStart != "2026-07-01" || repo.listRechargeQuery.DateEnd != "2026-07-31" {
		t.Fatalf("ListRecharges filters changed: %#v", repo.listRechargeQuery)
	}
	if repo.listRechargeCalls != 1 {
		t.Fatalf("ListRecharges must keep its independent query path, calls=%d", repo.listRechargeCalls)
	}
}

func TestCreateRechargeChoosesLowestSortEnabledAlipayConfig(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.packages = []RechargePackage{{ID: 1, Code: "recharge_10", Name: "¥10", AmountCents: 1000, Status: enum.CommonYes, IsDel: enum.CommonNo}}
	repo.configs = []Config{
		enabledRechargeConfig(1, "alipay_slow", 200, []string{enum.PaymentMethodWeb}),
		enabledRechargeConfig(2, "alipay_fast", 10, []string{enum.PaymentMethodWeb}),
	}
	gw := &fakeOrderGateway{payResult: &gateway.PayResult{PayURL: "https://pay.example.test"}}
	service := newRechargeService(repo, gw)

	result, appErr := service.CreateRecharge(context.Background(), RechargeCreateInput{
		UserID:      7,
		PackageCode: "recharge_10",
		PayMethod:   enum.PaymentMethodWeb,
		ReturnURL:   "https://example.test/payment/recharge",
	})
	if appErr != nil {
		t.Fatalf("CreateRecharge error=%v", appErr)
	}
	if result.Status != rechargeStatusPaying || repo.order.ConfigCode != "alipay_fast" || repo.recharge.AmountCents != 1000 {
		t.Fatalf("unexpected create result=%#v order=%#v recharge=%#v", result, repo.order, repo.recharge)
	}
	if strings.Contains(repo.recharge.RechargeNo, "alipay") || gw.payInput.AmountCents != 1000 {
		t.Fatalf("recharge must not be built from frontend config/amount, recharge=%#v pay=%#v", repo.recharge, gw.payInput)
	}
	if !strings.Contains(repo.order.ReturnURL, "tab=records") || !strings.Contains(repo.order.ReturnURL, "recharge_no="+repo.recharge.RechargeNo) {
		t.Fatalf("return_url must append recharge state, got %q", repo.order.ReturnURL)
	}
}

func TestRechargeNumbersKeepMillisecondDistinct(t *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

	if newPaymentRechargeNo(base) == newPaymentRechargeNo(base.Add(time.Millisecond)) {
		t.Fatalf("recharge numbers must differ across millisecond-separated timestamps: %s", newPaymentRechargeNo(base))
	}
	firstRecharge := newPaymentRechargeNo(base)
	secondRecharge := newPaymentRechargeNo(base)
	if firstRecharge == secondRecharge {
		t.Fatalf("recharge numbers must differ across repeated calls at the same timestamp")
	}
}

func TestCreateRechargeRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		input RechargeCreateInput
		want  string
		setup func(*fakeRechargeRepo)
	}{
		{name: "empty user", input: RechargeCreateInput{PackageCode: "recharge_10", PayMethod: enum.PaymentMethodWeb, ReturnURL: "https://example.test"}, want: "Token"},
		{name: "missing package", input: RechargeCreateInput{UserID: 7, PackageCode: "missing", PayMethod: enum.PaymentMethodWeb, ReturnURL: "https://example.test"}, want: "套餐"},
		{name: "missing config", input: RechargeCreateInput{UserID: 7, PackageCode: "recharge_10", PayMethod: enum.PaymentMethodH5, ReturnURL: "https://example.test"}, want: "支付配置", setup: func(repo *fakeRechargeRepo) {
			repo.packages = []RechargePackage{{ID: 1, Code: "recharge_10", Name: "¥10", AmountCents: 1000, Status: enum.CommonYes, IsDel: enum.CommonNo}}
			repo.configs = []Config{enabledRechargeConfig(1, "alipay_web", 10, []string{enum.PaymentMethodWeb})}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRechargeRepo()
			if tt.setup != nil {
				tt.setup(repo)
			}
			service := newRechargeService(repo, &fakeOrderGateway{})
			_, appErr := service.CreateRecharge(context.Background(), tt.input)
			if appErr == nil || !strings.Contains(appErr.Message, tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, appErr)
			}
		})
	}
}

func TestSyncRechargeReturnsCreditedWithoutCreditingAgain(t *testing.T) {
	repo := newFakeRechargeRepo()
	now := fixedRechargeNow()
	repo.wallet = &Wallet{ID: 1, UserID: 7, BalanceUnits: 1000, TotalRechargeUnits: 1000, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260515100000000000", Status: orderStatusPaid, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusCredited, AmountCents: 1000, PaidAt: &now, CreditedAt: &now, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.SyncRecharge(context.Background(), 7, 1)
	if appErr != nil {
		t.Fatalf("SyncRecharge error=%v", appErr)
	}
	if result.Status != rechargeStatusCredited || repo.creditCount != 0 {
		t.Fatalf("credited sync must be idempotent, result=%#v creditCount=%d", result, repo.creditCount)
	}
}

func TestSyncRechargeFinalizesPaidRechargeThroughAtomicEntry(t *testing.T) {
	repo := newFakeRechargeRepo()
	paidAt := fixedRechargeNow().Add(-time.Minute)
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260515100000000000", AmountCents: 1000, Status: orderStatusPaid, PaidAt: &paidAt, AlipayTradeNo: "202605152200", IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaid, AmountCents: 1000, PaidAt: &paidAt, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.SyncRecharge(context.Background(), 7, 1)
	if appErr != nil {
		t.Fatalf("SyncRecharge error=%v", appErr)
	}
	if result.Status != rechargeStatusCredited || repo.finalizeCount != 1 || repo.creditCount != 1 || repo.wallet.BalanceUnits != 1000*1_000_000 {
		t.Fatalf("sync must use atomic finalizer exactly once, result=%#v finalize=%d credit=%d wallet=%#v", result, repo.finalizeCount, repo.creditCount, repo.wallet)
	}
}

func TestSyncRechargeStaleReplayAfterCallbackDoesNotDoubleCredit(t *testing.T) {
	repo := newFakeRechargeRepo()
	paidAt := fixedRechargeNow().Add(-time.Minute)
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260515100000000000", AmountCents: 1000, Status: orderStatusPaid, PaidAt: &paidAt, AlipayTradeNo: "202605152200", IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaid, AmountCents: 1000, PaidAt: &paidAt, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})
	stale := repo.withOrder()

	if _, appErr := service.FinalizeOrderPaid(context.Background(), 1, "202605152200", paidAt, finalizeSourceCallback); appErr != nil {
		t.Fatalf("callback finalization error=%v", appErr)
	}
	result, appErr := service.syncRechargeRow(context.Background(), stale)
	if appErr != nil {
		t.Fatalf("stale sync replay error=%v", appErr)
	}
	if result.Status != rechargeStatusCredited || repo.finalizeCount != 2 || repo.creditCount != 1 || repo.wallet.BalanceUnits != 1000*1_000_000 {
		t.Fatalf("callback/sync race must converge, result=%#v finalize=%d credit=%d wallet=%#v", result, repo.finalizeCount, repo.creditCount, repo.wallet)
	}
}

func TestPayRechargeStaleThreadConvergesAfterCallbackCredit(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{*enabledOrderConfig()}
	repo.order = &Order{
		ID: 1, OrderNo: "PAY20260515100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay,
		PayMethod: enum.PaymentMethodWeb, Subject: "余额充值", AmountCents: 1000, Status: orderStatusPending,
		ReturnURL: "https://example.test/payment/recharge", ExpiredAt: fixedRechargeNow().Add(time.Hour), IsDel: enum.CommonNo,
	}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPending, AmountCents: 1000, IsDel: enum.CommonNo}
	repo.beforeUpdateOrderPaying = func() {
		paidAt := fixedRechargeNow().Add(-time.Second)
		creditedAt := fixedRechargeNow()
		repo.order.Status = orderStatusPaid
		repo.order.PaidAt = &paidAt
		repo.order.AlipayTradeNo = "202605152200"
		repo.recharge.Status = rechargeStatusCredited
		repo.recharge.PaidAt = &paidAt
		repo.recharge.CreditedAt = &creditedAt
		repo.wallet.BalanceUnits = 1000 * 1_000_000
		repo.wallet.TotalRechargeUnits = 1000 * 1_000_000
		repo.creditCount++
	}
	service := newRechargeService(repo, &fakeOrderGateway{payResult: &gateway.PayResult{PayURL: "https://pay.example.test"}})

	result, appErr := service.PayRecharge(context.Background(), 7, 1)
	if appErr != nil || result == nil || result.Status != rechargeStatusCredited {
		t.Fatalf("stale pay thread must converge to credited fact, result=%#v err=%v", result, appErr)
	}
	if repo.recharge.Status != rechargeStatusCredited || repo.recharge.CreditedAt == nil || repo.creditCount != 1 {
		t.Fatalf("stale pay thread must not downgrade or re-credit, recharge=%#v credits=%d", repo.recharge, repo.creditCount)
	}
}

func TestPayRechargePayingWithoutURLConvergesAfterRechargeCASMiss(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{*enabledOrderConfig()}
	repo.order = &Order{
		ID: 1, OrderNo: "PAY20260515100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay,
		PayMethod: enum.PaymentMethodWeb, Subject: "余额充值", AmountCents: 1000, Status: orderStatusPending,
		ReturnURL: "https://example.test/payment/recharge", ExpiredAt: fixedRechargeNow().Add(time.Hour), IsDel: enum.CommonNo,
	}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{payResult: &gateway.PayResult{PayURL: "https://pay.example.test/reentry"}})

	result, appErr := service.PayRecharge(context.Background(), 7, 1)
	if appErr != nil {
		t.Fatalf("paying recharge reentry error=%v", appErr)
	}
	if result == nil || result.Status != rechargeStatusPaying || result.PayURL != "https://pay.example.test/reentry" {
		t.Fatalf("recharge CAS miss must reload current payment fact, result=%#v", result)
	}
	if repo.recharge.Status != rechargeStatusPaying || repo.order.Status != orderStatusPaying || repo.order.PayURL != result.PayURL {
		t.Fatalf("reentry must preserve paying recharge and expose persisted order URL, recharge=%#v order=%#v", repo.recharge, repo.order)
	}
}

func TestPayRechargeGatewayFailureConvergesAfterCallbackCredit(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{*enabledOrderConfig()}
	repo.order = &Order{
		ID: 1, OrderNo: "PAY20260515100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay,
		PayMethod: enum.PaymentMethodWeb, Subject: "余额充值", AmountCents: 1000, Status: orderStatusPending,
		ReturnURL: "https://example.test/payment/recharge", ExpiredAt: fixedRechargeNow().Add(time.Hour), IsDel: enum.CommonNo,
	}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPending, AmountCents: 1000, IsDel: enum.CommonNo}
	repo.beforeUpdateOrderFailed = func() {
		paidAt := fixedRechargeNow().Add(-time.Second)
		creditedAt := fixedRechargeNow()
		repo.order.Status = orderStatusPaid
		repo.order.PaidAt = &paidAt
		repo.order.AlipayTradeNo = "202605152200"
		repo.recharge.Status = rechargeStatusCredited
		repo.recharge.PaidAt = &paidAt
		repo.recharge.CreditedAt = &creditedAt
		repo.wallet.BalanceUnits = 1000 * 1_000_000
		repo.wallet.TotalRechargeUnits = 1000 * 1_000_000
		repo.creditCount++
	}
	service := newRechargeService(repo, &fakeOrderGateway{payErr: errors.New("gateway down")})

	result, appErr := service.PayRecharge(context.Background(), 7, 1)
	if appErr != nil || result == nil || result.Status != rechargeStatusCredited {
		t.Fatalf("gateway failure thread must converge to callback credit, result=%#v err=%v", result, appErr)
	}
	if repo.recharge.Status != rechargeStatusCredited || repo.order.Status != orderStatusPaid || repo.creditCount != 1 {
		t.Fatalf("gateway failure must not overwrite callback terminal facts, recharge=%#v order=%#v credits=%d", repo.recharge, repo.order, repo.creditCount)
	}
}

func TestPayRechargeLateGatewayFailurePreservesPayingAndFinalizesOnce(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{*enabledOrderConfig()}
	repo.order = &Order{
		ID: 1, OrderNo: "PAY20260515100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay,
		PayMethod: enum.PaymentMethodWeb, Subject: "余额充值", AmountCents: 1000, Status: orderStatusPending,
		ReturnURL: "https://example.test/payment/recharge", ExpiredAt: fixedRechargeNow().Add(time.Hour), IsDel: enum.CommonNo,
	}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPending, AmountCents: 1000, IsDel: enum.CommonNo}
	persistedURL := "https://pay.example.test/winner"
	repo.beforeUpdateOrderFailed = func() {
		repo.order.Status = orderStatusPaying
		repo.order.PayURL = persistedURL
		repo.order.FailureReason = ""
		repo.recharge.Status = rechargeStatusPaying
		repo.recharge.FailureReason = ""
	}
	service := newRechargeService(repo, &fakeOrderGateway{payErr: errors.New("gateway down")})

	result, appErr := service.PayRecharge(context.Background(), 7, 1)
	if appErr != nil || result == nil || result.Status != rechargeStatusPaying || result.PayURL != persistedURL {
		t.Fatalf("late gateway failure must converge to persisted recharge URL, result=%#v err=%v", result, appErr)
	}
	if repo.order.Status != orderStatusPaying || repo.order.PayURL != persistedURL || repo.recharge.Status != rechargeStatusPaying {
		t.Fatalf("late gateway failure downgraded winning payment facts, order=%#v recharge=%#v", repo.order, repo.recharge)
	}

	paidAt := fixedRechargeNow().Add(time.Second)
	if _, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605152200", paidAt, finalizeSourceCallback); appErr != nil {
		t.Fatalf("callback finalization after payment race failed: %v", appErr)
	}
	if _, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605152200", paidAt, finalizeSourceCallback); appErr != nil {
		t.Fatalf("callback replay after payment race failed: %v", appErr)
	}
	if repo.order.Status != orderStatusPaid || repo.recharge.Status != rechargeStatusCredited || repo.creditCount != 1 || repo.finalizeCount != 2 {
		t.Fatalf("payment race must finalize and credit once, order=%#v recharge=%#v credits=%d finalizes=%d", repo.order, repo.recharge, repo.creditCount, repo.finalizeCount)
	}
}

func TestCloseRechargeRejectsCredited(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260515100000000000", Status: orderStatusPaid, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260515100000000000", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusCredited, AmountCents: 1000, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	_, appErr := service.CloseRecharge(context.Background(), 7, 1)
	if appErr == nil || !strings.Contains(appErr.Message, "不能关闭") {
		t.Fatalf("expected credited close rejection, got %v", appErr)
	}
}

func newRechargeService(repo *fakeRechargeRepo, gw *fakeOrderGateway) *Service {
	return NewService(Dependencies{
		Repository:   repo,
		Gateway:      gw,
		Secretbox:    &fakeSecretbox{},
		CertResolver: fakeResolver{},
		CertStore:    &fakeCertStore{},
		Now:          fixedRechargeNow,
	})
}

func fixedRechargeNow() time.Time { return time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC) }

func enabledRechargeConfig(id int64, code string, sort int, methods []string) Config {
	cfg := *enabledOrderConfig()
	cfg.ID = id
	cfg.Code = code
	cfg.Sort = sort
	cfg.EnabledMethodsJSON = mustConfigJSON(methods)
	return cfg
}

type fakeRechargeRepo struct {
	packages                  []RechargePackage
	configs                   []Config
	wallet                    *Wallet
	order                     *Order
	recharge                  *Recharge
	rechargeByOrder           map[int64]*Recharge
	batchOrders               []Order
	callbackEvent             CallbackEvent
	callbackCreateErr         error
	rejectInvalidCallbackJSON bool
	creditCount               int
	finalizeCount             int
	beforeFinalizePaidOrder   func(paidAt time.Time)
	beforeUpdateOrderPaying   func()
	beforeUpdateOrderFailed   func()
	listRechargeQuery         RechargeListQuery
	listRechargeCalls         int
}

func newFakeRechargeRepo() *fakeRechargeRepo {
	return &fakeRechargeRepo{wallet: &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}}
}

func (r *fakeRechargeRepo) ListConfigs(ctx context.Context, query ConfigListQuery) ([]Config, int64, error) {
	return r.configs, int64(len(r.configs)), nil
}
func (r *fakeRechargeRepo) GetConfig(ctx context.Context, id int64) (*Config, error) {
	for idx := range r.configs {
		if r.configs[idx].ID == id {
			copy := r.configs[idx]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *fakeRechargeRepo) GetConfigByCode(ctx context.Context, code string) (*Config, error) {
	for idx := range r.configs {
		if r.configs[idx].Code == strings.TrimSpace(code) {
			copy := r.configs[idx]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *fakeRechargeRepo) GetConfigByIDForSettlement(ctx context.Context, id int64) (*Config, error) {
	for idx := range r.configs {
		if r.configs[idx].ID == id {
			copy := r.configs[idx]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *fakeRechargeRepo) CreateConfig(ctx context.Context, cfg Config) (int64, error) {
	return 0, nil
}
func (r *fakeRechargeRepo) UpdateConfig(ctx context.Context, cfg Config, keepPrivateKey bool) error {
	return nil
}
func (r *fakeRechargeRepo) ChangeConfigStatus(ctx context.Context, id int64, status int) error {
	return nil
}
func (r *fakeRechargeRepo) DeleteConfig(ctx context.Context, id int64) error { return nil }
func (r *fakeRechargeRepo) ListRechargePackages(ctx context.Context) ([]RechargePackage, error) {
	return r.packages, nil
}
func (r *fakeRechargeRepo) GetRechargePackageByCode(ctx context.Context, code string) (*RechargePackage, error) {
	for idx := range r.packages {
		if r.packages[idx].Code == strings.TrimSpace(code) {
			copy := r.packages[idx]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *fakeRechargeRepo) GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error) {
	if r.wallet == nil {
		r.wallet = &Wallet{ID: 1, UserID: userID, IsDel: enum.CommonNo}
	}
	return r.wallet, nil
}
func (r *fakeRechargeRepo) GetWallet(ctx context.Context, userID int64) (*Wallet, error) {
	return r.wallet, nil
}
func (r *fakeRechargeRepo) ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error) {
	if r.order == nil {
		return nil, 0, nil
	}
	return []Order{*r.order}, 1, nil
}
func (r *fakeRechargeRepo) GetOrder(ctx context.Context, id int64) (*Order, error) {
	if r.order == nil || r.order.ID != id {
		return nil, nil
	}
	copy := *r.order
	return &copy, nil
}

func (r *fakeRechargeRepo) GetOrderByNo(ctx context.Context, orderNo string) (*Order, error) {
	if r.order != nil && r.order.OrderNo == strings.TrimSpace(orderNo) {
		copy := *r.order
		return &copy, nil
	}
	for idx := range r.batchOrders {
		if r.batchOrders[idx].OrderNo == strings.TrimSpace(orderNo) {
			copy := r.batchOrders[idx]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *fakeRechargeRepo) ListPendingPayingOrders(ctx context.Context, cutoff time.Time, limit int) ([]Order, error) {
	rows := make([]Order, 0, len(r.batchOrders))
	for _, row := range r.batchOrders {
		if row.Status == orderStatusPaying {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (r *fakeRechargeRepo) ListUncreditedPaidRecharges(ctx context.Context, limit int) ([]RechargeWithOrder, error) {
	rows := make([]RechargeWithOrder, 0, len(r.rechargeByOrder))
	for orderID, recharge := range r.rechargeByOrder {
		if recharge == nil || recharge.CreditedAt != nil || recharge.Status == rechargeStatusCredited || recharge.Status == rechargeStatusClosed || recharge.Status == rechargeStatusFailed {
			continue
		}
		var order *Order
		for idx := range r.batchOrders {
			if r.batchOrders[idx].ID == orderID {
				order = &r.batchOrders[idx]
				break
			}
		}
		if order == nil || order.Status != orderStatusPaid {
			continue
		}
		r.recharge = recharge
		row := RechargeWithOrder{Recharge: *recharge}
		row.PaymentOrderNo = order.OrderNo
		row.OrderStatus = order.Status
		row.AlipayTradeNo = order.AlipayTradeNo
		row.OrderPaidAt = order.PaidAt
		rows = append(rows, row)
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return rows, nil
}
func (r *fakeRechargeRepo) ListExpiredOpenOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	rows := make([]Order, 0, len(r.batchOrders))
	for _, row := range r.batchOrders {
		if row.Status == orderStatusPending || row.Status == orderStatusPaying {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (r *fakeRechargeRepo) CreateOrder(ctx context.Context, order Order) (int64, error) {
	order.ID = 1
	r.order = &order
	return order.ID, nil
}
func (r *fakeRechargeRepo) UpdateOrderPaying(ctx context.Context, id int64, payURL string) error {
	if r.beforeUpdateOrderPaying != nil {
		r.beforeUpdateOrderPaying()
		r.beforeUpdateOrderPaying = nil
	}
	if r.order == nil || r.order.ID != id || (r.order.Status != orderStatusPending && r.order.Status != orderStatusFailed) {
		return ErrPaymentStateChanged
	}
	r.order.Status = orderStatusPaying
	r.order.PayURL = payURL
	return nil
}
func (r *fakeRechargeRepo) UpdateOrderFailed(ctx context.Context, id int64, reason string) error {
	if r.beforeUpdateOrderFailed != nil {
		r.beforeUpdateOrderFailed()
		r.beforeUpdateOrderFailed = nil
	}
	if r.order == nil || r.order.ID != id || (r.order.Status != orderStatusPending && r.order.Status != orderStatusFailed) {
		return ErrPaymentStateChanged
	}
	r.order.Status = orderStatusFailed
	r.order.FailureReason = reason
	return nil
}
func (r *fakeRechargeRepo) UpdateOrderClosed(ctx context.Context, id int64, closedAt time.Time) error {
	order := r.findOrderRef(id)
	if order == nil {
		return nil
	}
	if order.Status != orderStatusPending && order.Status != orderStatusFailed && order.Status != orderStatusPaying {
		return nil
	}
	order.Status = orderStatusClosed
	order.ClosedAt = &closedAt
	return nil
}
func (r *fakeRechargeRepo) ListEnabledOrderConfigOptions(ctx context.Context) ([]Config, error) {
	return r.configs, nil
}
func (r *fakeRechargeRepo) ListRecharges(ctx context.Context, query RechargeListQuery) ([]RechargeWithOrder, int64, error) {
	r.listRechargeCalls++
	r.listRechargeQuery = query
	if r.recharge == nil {
		return nil, 0, nil
	}
	return []RechargeWithOrder{r.withOrder()}, 1, nil
}
func (r *fakeRechargeRepo) GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeWithOrder, error) {
	if r.recharge == nil || r.recharge.ID != id || r.recharge.UserID != userID {
		return nil, nil
	}
	row := r.withOrder()
	return &row, nil
}

func (r *fakeRechargeRepo) GetRechargeByOrderID(ctx context.Context, orderID int64) (*Recharge, error) {
	if r.recharge != nil && r.recharge.PaymentOrderID == orderID {
		return r.recharge, nil
	}
	if r.rechargeByOrder != nil && r.rechargeByOrder[orderID] != nil {
		r.recharge = r.rechargeByOrder[orderID]
		return r.recharge, nil
	}
	return nil, nil
}
func (r *fakeRechargeRepo) CreateRechargeWithOrder(ctx context.Context, recharge Recharge, order Order) (RechargeWithOrder, error) {
	order.ID = 1
	recharge.ID = 1
	recharge.PaymentOrderID = order.ID
	r.order = &order
	r.recharge = &recharge
	return r.withOrder(), nil
}
func (r *fakeRechargeRepo) UpdateRechargePaying(ctx context.Context, id int64) error {
	if r.recharge == nil || r.recharge.ID != id || (r.recharge.Status != rechargeStatusPending && r.recharge.Status != rechargeStatusFailed) {
		return ErrPaymentStateChanged
	}
	r.recharge.Status = rechargeStatusPaying
	r.recharge.FailureReason = ""
	return nil
}
func (r *fakeRechargeRepo) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	if r.recharge == nil || r.recharge.ID != id || (r.recharge.Status != rechargeStatusPending && r.recharge.Status != rechargeStatusFailed) {
		return ErrPaymentStateChanged
	}
	r.recharge.Status = rechargeStatusFailed
	r.recharge.FailureReason = reason
	return nil
}
func (r *fakeRechargeRepo) UpdateRechargeClosed(ctx context.Context, id int64) error {
	if !canCloseLinkedRecharge(r.recharge.Status) {
		return nil
	}
	r.recharge.Status = rechargeStatusClosed
	return nil
}

func (r *fakeRechargeRepo) FinalizePaidOrder(_ context.Context, orderID int64, tradeNo string, paidAt time.Time, now time.Time) (*PaidOrderFinalization, error) {
	r.finalizeCount++
	order := r.findOrderRef(orderID)
	if order == nil {
		return nil, ErrPaymentOrderNotFound
	}
	if order.Status != orderStatusPending && order.Status != orderStatusPaying && order.Status != orderStatusPaid {
		return nil, ErrPaymentStateChanged
	}
	if r.beforeFinalizePaidOrder != nil {
		r.beforeFinalizePaidOrder(paidAt)
		r.beforeFinalizePaidOrder = nil
	}
	alreadyPaid := order.Status == orderStatusPaid
	recharge := r.recharge
	if recharge == nil || recharge.PaymentOrderID != orderID {
		recharge = r.rechargeByOrder[orderID]
	}
	if recharge == nil {
		if !alreadyPaid {
			order.Status = orderStatusPaid
			order.AlipayTradeNo = tradeNo
			order.PaidAt = &paidAt
		}
		return &PaidOrderFinalization{Order: order, OrderPaid: !alreadyPaid, OrderAlreadyPaid: alreadyPaid, RawOrder: true}, nil
	}
	if recharge.PaymentOrderID != order.ID || recharge.AmountCents != order.AmountCents || recharge.UserID <= 0 {
		return nil, ErrPaymentStateChanged
	}
	credited := recharge.Status == rechargeStatusCredited && recharge.CreditedAt != nil
	if (recharge.Status == rechargeStatusCredited) != (recharge.CreditedAt != nil) || recharge.Status == rechargeStatusClosed || recharge.Status == rechargeStatusFailed ||
		(recharge.Status == rechargeStatusCredited && (!alreadyPaid || recharge.PaidAt == nil)) ||
		(recharge.Status == rechargeStatusPaid && (!alreadyPaid || recharge.PaidAt == nil)) ||
		((recharge.Status == rechargeStatusPending || recharge.Status == rechargeStatusPaying) && recharge.PaidAt != nil) {
		return nil, ErrPaymentStateChanged
	}
	if !alreadyPaid {
		order.Status = orderStatusPaid
		order.AlipayTradeNo = tradeNo
		order.PaidAt = &paidAt
	}
	if !credited {
		if r.wallet == nil || r.wallet.UserID != recharge.UserID {
			return nil, ErrPaymentStateChanged
		}
		units := recharge.AmountCents * 1_000_000
		r.creditCount++
		r.wallet.BalanceUnits += units
		r.wallet.TotalRechargeUnits += units
		recharge.Status, recharge.PaidAt, recharge.CreditedAt = rechargeStatusCredited, &paidAt, &now
	}
	r.recharge = recharge
	return &PaidOrderFinalization{
		Order:                   order,
		Recharge:                recharge,
		Wallet:                  r.wallet,
		OrderPaid:               !alreadyPaid,
		OrderAlreadyPaid:        alreadyPaid,
		RechargeCredited:        !credited,
		RechargeAlreadyCredited: credited,
	}, nil
}
func (r *fakeRechargeRepo) FirstEnabledConfigForPay(ctx context.Context, provider string, payMethod string) (*Config, error) {
	var selected *Config
	for idx := range r.configs {
		row := r.configs[idx]
		if row.Provider != provider || row.Status != enum.CommonYes || row.IsDel != enum.CommonNo || !methodEnabled(row.EnabledMethodsJSON, payMethod) {
			continue
		}
		if selected == nil || row.Sort < selected.Sort || (row.Sort == selected.Sort && row.ID < selected.ID) {
			copy := row
			selected = &copy
		}
	}
	return selected, nil
}
func (r *fakeRechargeRepo) withOrder() RechargeWithOrder {
	row := RechargeWithOrder{Recharge: *r.recharge}
	if r.order != nil {
		row.PaymentOrderNo = r.order.OrderNo
		row.PayURL = r.order.PayURL
		row.OrderStatus = r.order.Status
		row.AlipayTradeNo = r.order.AlipayTradeNo
		row.OrderPaidAt = r.order.PaidAt
	}
	return row
}

func (r *fakeRechargeRepo) findOrderRef(id int64) *Order {
	if r.order != nil && r.order.ID == id {
		return r.order
	}
	for idx := range r.batchOrders {
		if r.batchOrders[idx].ID == id {
			return &r.batchOrders[idx]
		}
	}
	return nil
}
