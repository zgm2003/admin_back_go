package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	gateway "admin_back_go/internal/infra/payment"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/enum"
)

func TestNewPaymentSyncPendingOrderTaskUsesVersionedType(t *testing.T) {
	task, err := NewSyncPendingOrderTask(SyncPendingOrderPayload{Limit: 9})
	if err != nil {
		t.Fatalf("NewSyncPendingOrderTask error=%v", err)
	}
	if task.Type != TypeSyncPendingOrderV1 || task.Queue != taskqueue.QueueDefault || task.UniqueTTL != 55*time.Second {
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
	if result.Scanned != 2 || result.Paid != 1 || result.Failed != 1 || repo.creditCount != 1 {
		t.Fatalf("unexpected result=%#v creditCount=%d", result, repo.creditCount)
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
	if result.Scanned != 2 || result.Closed != 1 || result.Paid != 1 || repo.creditCount != 1 {
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
