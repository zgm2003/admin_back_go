package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	gateway "admin_back_go/internal/infra/payment"
	"admin_back_go/internal/shared/enum"
)

func TestNewPaymentSyncPendingOrderTaskUsesVersionedType(t *testing.T) {
	task, err := NewSyncPendingOrderTask(SyncPendingOrderPayload{Limit: 9})
	if err != nil {
		t.Fatalf("NewSyncPendingOrderTask error=%v", err)
	}
	if task.Type != TypeSyncPendingOrderV1 || task.Queue != "" || task.UniqueTTL != 0 {
		t.Fatalf("unexpected task=%#v", task)
	}
	payload, err := DecodeSyncPendingOrderPayload(task.Payload)
	if err != nil {
		t.Fatalf("DecodeSyncPendingOrderPayload error=%v", err)
	}
	if payload.Limit != 9 {
		t.Fatalf("expected limit 9, got %#v", payload)
	}
}

func TestSyncPendingOrdersCreditsPaidAndContinuesAfterFailures(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	paidAt := fixedRechargeNow().Add(time.Minute)
	repo.batchOrders = []Order{
		{ID: 1, OrderNo: "PAY20260521000000000001", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo},
		{ID: 2, OrderNo: "PAY20260521000000000002", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo},
	}
	repo.order = &repo.batchOrders[0]
	repo.rechargeByOrder = map[int64]*Recharge{1: {ID: 10, RechargeNo: "RCG1", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}}
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	gw := &fakeOrderGateway{queryResults: map[string]*gateway.QueryResult{
		"PAY20260521000000000001": {Status: "TRADE_SUCCESS", TradeNo: "202605212200", PaidAt: &paidAt},
	}, queryErrors: map[string]error{"PAY20260521000000000002": errors.New("alipay timeout")}}
	service := newRechargeService(repo, gw)

	result, err := service.SyncPendingOrders(context.Background(), SyncPendingOrderInput{Limit: 2})
	if err != nil {
		t.Fatalf("SyncPendingOrders error=%v", err)
	}
	if result.Scanned != 2 || result.Paid != 1 || result.Failed != 1 || repo.creditCount != 1 || repo.finalizeCount != 1 {
		t.Fatalf("unexpected result=%#v creditCount=%d", result, repo.creditCount)
	}
}

func TestSyncPendingOrdersCreditsPreviouslyPaidUncreditedRecharge(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	paidAt := fixedRechargeNow().Add(time.Minute)
	repo.batchOrders = []Order{
		{ID: 1, OrderNo: "PAY20260521000000000001", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaid, PaidAt: &paidAt, IsDel: enum.CommonNo},
	}
	repo.order = &repo.batchOrders[0]
	repo.rechargeByOrder = map[int64]*Recharge{
		1: {ID: 10, RechargeNo: "RCG1", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaid, AmountCents: 1000, PaidAt: &paidAt, IsDel: enum.CommonNo},
	}
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, err := service.SyncPendingOrders(context.Background(), SyncPendingOrderInput{Limit: 1})
	if err != nil {
		t.Fatalf("SyncPendingOrders error=%v", err)
	}
	if result.Scanned != 1 || result.Paid != 1 || result.Failed != 0 || repo.creditCount != 1 || repo.finalizeCount != 1 {
		t.Fatalf("expected paid uncredited recharge to be compensated, result=%#v creditCount=%d", result, repo.creditCount)
	}
	if repo.rechargeByOrder[1].Status != rechargeStatusCredited || repo.rechargeByOrder[1].CreditedAt == nil {
		t.Fatalf("expected credited recharge, got %#v", repo.rechargeByOrder[1])
	}
}

func TestCreditUncreditedPaidRechargeStaleReplayUsesAtomicFinalizer(t *testing.T) {
	repo := newFakeRechargeRepo()
	paidAt := fixedRechargeNow().Add(-time.Minute)
	repo.batchOrders = []Order{{ID: 1, OrderNo: "PAY20260521000000000001", AmountCents: 1000, Status: orderStatusPaid, PaidAt: &paidAt, AlipayTradeNo: "202605212200", IsDel: enum.CommonNo}}
	repo.order = &repo.batchOrders[0]
	repo.rechargeByOrder = map[int64]*Recharge{1: {ID: 10, RechargeNo: "RCG1", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaid, AmountCents: 1000, PaidAt: &paidAt, IsDel: enum.CommonNo}}
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})
	stale := RechargeWithOrder{Recharge: *repo.rechargeByOrder[1], AlipayTradeNo: repo.order.AlipayTradeNo, OrderPaidAt: repo.order.PaidAt}

	for attempt := 0; attempt < 2; attempt++ {
		outcome, err := service.creditUncreditedPaidRecharge(context.Background(), stale)
		if err != nil || outcome != paymentJobOutcomePaid {
			t.Fatalf("cron replay attempt %d outcome=%q err=%v", attempt+1, outcome, err)
		}
	}
	if repo.finalizeCount != 2 || repo.creditCount != 1 || repo.wallet.BalanceUnits != 1000*1_000_000 {
		t.Fatalf("cron stale replay must converge through atomic finalizer, finalize=%d credit=%d wallet=%#v", repo.finalizeCount, repo.creditCount, repo.wallet)
	}
}

func TestCloseExpiredOrdersClosesPendingAndFinalizesPaidPaying(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	paidAt := fixedRechargeNow().Add(time.Minute)
	repo.batchOrders = []Order{
		{ID: 1, OrderNo: "PAY20260521000000000001", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPending, ExpiredAt: fixedRechargeNow().Add(-time.Minute), IsDel: enum.CommonNo},
		{ID: 2, OrderNo: "PAY20260521000000000002", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, ExpiredAt: fixedRechargeNow().Add(-time.Minute), IsDel: enum.CommonNo},
	}
	repo.order = &repo.batchOrders[1]
	repo.rechargeByOrder = map[int64]*Recharge{2: {ID: 20, RechargeNo: "RCG2", UserID: 7, PaymentOrderID: 2, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}}
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	gw := &fakeOrderGateway{queryResults: map[string]*gateway.QueryResult{
		"PAY20260521000000000002": {Status: "TRADE_SUCCESS", TradeNo: "202605212200", PaidAt: &paidAt},
	}}
	service := newRechargeService(repo, gw)

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{Limit: 2})
	if err != nil {
		t.Fatalf("CloseExpiredOrders error=%v", err)
	}
	if result.Scanned != 2 || result.Closed != 1 || result.Paid != 1 || repo.creditCount != 1 || repo.finalizeCount != 1 {
		t.Fatalf("unexpected result=%#v creditCount=%d", result, repo.creditCount)
	}
}

func TestCloseExpiredOrdersClosesMissingAlipayTradeLocally(t *testing.T) {
	repo := newFakeRechargeRepo()
	repo.configs = []Config{enabledRechargeConfig(1, "alipay_default", 1, []string{enum.PaymentMethodWeb})}
	repo.batchOrders = []Order{
		{ID: 1, OrderNo: "PAY20260521000000000001", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, ExpiredAt: fixedRechargeNow().Add(-time.Minute), IsDel: enum.CommonNo},
	}
	repo.order = &repo.batchOrders[0]
	repo.rechargeByOrder = map[int64]*Recharge{1: {ID: 10, RechargeNo: "RCG1", UserID: 7, PaymentOrderID: 1, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}}
	gw := &fakeOrderGateway{queryErrors: map[string]error{
		"PAY20260521000000000001": errors.New(`alipay: query: {"sub_code":"ACQ.TRADE_NOT_EXIST","sub_msg":"交易不存在"}`),
	}}
	service := newRechargeService(repo, gw)

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{Limit: 1})
	if err != nil {
		t.Fatalf("CloseExpiredOrders error=%v", err)
	}
	if result.Scanned != 1 || result.Closed != 1 || result.Failed != 0 || repo.order.Status != orderStatusClosed || repo.rechargeByOrder[1].Status != rechargeStatusClosed {
		t.Fatalf("expected missing Alipay trade to close locally, result=%#v order=%#v recharge=%#v", result, repo.order, repo.rechargeByOrder[1])
	}
}
