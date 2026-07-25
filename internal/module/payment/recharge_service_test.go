package payment

import (
	"context"
	"strings"
	"testing"
	"time"

	gateway "admin_back_go/internal/infra/payment"
	"admin_back_go/internal/shared/enum"
)

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
	if newWalletTransactionNo(base) == newWalletTransactionNo(base.Add(time.Millisecond)) {
		t.Fatalf("wallet transaction numbers must differ across millisecond-separated timestamps: %s", newWalletTransactionNo(base))
	}
	firstRecharge := newPaymentRechargeNo(base)
	secondRecharge := newPaymentRechargeNo(base)
	if firstRecharge == secondRecharge {
		t.Fatalf("recharge numbers must differ across repeated calls at the same timestamp")
	}
	firstTransaction := newWalletTransactionNo(base)
	secondTransaction := newWalletTransactionNo(base)
	if firstTransaction == secondTransaction {
		t.Fatalf("wallet transaction numbers must differ across repeated calls at the same timestamp")
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
	beforeUpdateRechargePaid  func(paidAt time.Time)
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
	if r.order.Status != orderStatusPending && r.order.Status != orderStatusFailed {
		return nil
	}
	r.order.Status = orderStatusPaying
	r.order.PayURL = payURL
	return nil
}
func (r *fakeRechargeRepo) UpdateOrderFailed(ctx context.Context, id int64, reason string) error {
	if r.order.Status != orderStatusPending && r.order.Status != orderStatusFailed {
		return nil
	}
	r.order.Status = orderStatusFailed
	r.order.FailureReason = reason
	return nil
}
func (r *fakeRechargeRepo) UpdateOrderPaid(ctx context.Context, id int64, tradeNo string, paidAt time.Time) error {
	order := r.findOrderRef(id)
	if order == nil {
		return nil
	}
	if order.Status != orderStatusPending && order.Status != orderStatusPaying && order.Status != orderStatusPaid {
		return nil
	}
	order.Status = orderStatusPaid
	order.AlipayTradeNo = tradeNo
	order.PaidAt = &paidAt
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
	if r.recharge == nil {
		return nil, 0, nil
	}
	return []RechargeWithOrder{r.withOrder()}, 1, nil
}
func (r *fakeRechargeRepo) ListRecentRecharges(ctx context.Context, userID int64, limit int) ([]RechargeWithOrder, error) {
	if r.recharge == nil {
		return nil, nil
	}
	return []RechargeWithOrder{r.withOrder()}, nil
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
	r.recharge.Status = rechargeStatusPaying
	r.recharge.FailureReason = ""
	return nil
}
func (r *fakeRechargeRepo) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	r.recharge.Status = rechargeStatusFailed
	r.recharge.FailureReason = reason
	return nil
}
func (r *fakeRechargeRepo) UpdateRechargePaid(ctx context.Context, id int64, paidAt time.Time) error {
	if r.beforeUpdateRechargePaid != nil {
		r.beforeUpdateRechargePaid(paidAt)
		r.beforeUpdateRechargePaid = nil
	}
	if r.recharge.Status == rechargeStatusCredited || r.recharge.CreditedAt != nil {
		return nil
	}
	if r.recharge.Status != rechargeStatusPending && r.recharge.Status != rechargeStatusPaying && r.recharge.Status != rechargeStatusPaid {
		return nil
	}
	r.recharge.Status = rechargeStatusPaid
	r.recharge.PaidAt = &paidAt
	return nil
}
func (r *fakeRechargeRepo) UpdateRechargeClosed(ctx context.Context, id int64) error {
	if !canCloseLinkedRecharge(r.recharge.Status) {
		return nil
	}
	r.recharge.Status = rechargeStatusClosed
	return nil
}
func (r *fakeRechargeRepo) CreditRecharge(ctx context.Context, rechargeID int64, paidAt time.Time, now time.Time) (*Wallet, *Recharge, error) {
	if r.recharge == nil {
		for _, row := range r.rechargeByOrder {
			if row.ID == rechargeID {
				r.recharge = row
				break
			}
		}
	}
	if r.recharge.Status == rechargeStatusClosed || r.recharge.Status == rechargeStatusFailed {
		return nil, nil, ErrPaymentStateChanged
	}
	if r.recharge.Status == rechargeStatusCredited || r.recharge.CreditedAt != nil {
		r.recharge.Status = rechargeStatusCredited
		return r.wallet, r.recharge, nil
	}
	r.creditCount++
	r.wallet.BalanceUnits += r.recharge.AmountCents
	r.wallet.TotalRechargeUnits += r.recharge.AmountCents
	r.recharge.Status = rechargeStatusCredited
	r.recharge.PaidAt = &paidAt
	r.recharge.CreditedAt = &now
	return r.wallet, r.recharge, nil
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
