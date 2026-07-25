package payment

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"
)

func TestFinalizeOrderPaidCreditsRechargeOnce(t *testing.T) {
	repo := newFakeRechargeRepo()
	now := fixedRechargeNow()
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260521100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260521100000000000", UserID: 7, PaymentOrderID: repo.order.ID, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605212200", now, finalizeSourceCallback)
	if appErr != nil {
		t.Fatalf("FinalizeOrderPaid error=%v", appErr)
	}
	if !result.OrderPaid || !result.RechargeCredited || repo.order.Status != orderStatusPaid || repo.recharge.Status != rechargeStatusCredited || repo.creditCount != 1 {
		t.Fatalf("unexpected first finalize result=%#v order=%#v recharge=%#v creditCount=%d", result, repo.order, repo.recharge, repo.creditCount)
	}

	result, appErr = service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605212200", now.Add(time.Second), finalizeSourceCallback)
	if appErr != nil {
		t.Fatalf("duplicate FinalizeOrderPaid error=%v", appErr)
	}
	if !result.AlreadyPaid || !result.RechargeCredited || repo.creditCount != 1 || repo.wallet.BalanceUnits != 1000 {
		t.Fatalf("duplicate finalize must be idempotent result=%#v wallet=%#v creditCount=%d", result, repo.wallet, repo.creditCount)
	}
}

func TestFinalizeOrderPaidAllowsRawOrderWithoutRecharge(t *testing.T) {
	repo := newFakeOrderRepoWithOrder(orderStatusPaying)
	service := newOrderService(repo, &fakeOrderGateway{})

	result, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605212200", fixedOrderNow(), finalizeSourceSync)
	if appErr != nil {
		t.Fatalf("FinalizeOrderPaid raw order error=%v", appErr)
	}
	if !result.OrderPaid || result.RechargeCredited || repo.order.Status != orderStatusPaid {
		t.Fatalf("unexpected raw finalize result=%#v order=%#v", result, repo.order)
	}
}

func TestFinalizeOrderPaidDoesNotCreditWhenOrderPaidCASMisses(t *testing.T) {
	repo := newFakeRechargeRepo()
	now := fixedRechargeNow()
	closedAt := now.Add(-time.Minute)
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260521100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusClosed, ClosedAt: &closedAt, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260521100000000000", UserID: 7, PaymentOrderID: repo.order.ID, Status: rechargeStatusClosed, AmountCents: 1000, IsDel: enum.CommonNo}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605212200", now, finalizeSourceCallback)
	if appErr == nil {
		t.Fatalf("expected state conflict, got result=%#v", result)
	}
	if repo.creditCount != 0 || repo.wallet.BalanceUnits != 0 {
		t.Fatalf("CAS miss must not credit wallet, wallet=%#v creditCount=%d", repo.wallet, repo.creditCount)
	}
	if repo.order.Status != orderStatusClosed || repo.recharge.Status != rechargeStatusClosed {
		t.Fatalf("CAS miss must not reopen closed rows, order=%#v recharge=%#v", repo.order, repo.recharge)
	}
}

func TestFinalizeOrderPaidDoesNotDowngradeCreditedRechargeOnStaleSnapshot(t *testing.T) {
	repo := newFakeRechargeRepo()
	now := fixedRechargeNow()
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260521100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260521100000000000", UserID: 7, PaymentOrderID: repo.order.ID, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}
	repo.beforeUpdateRechargePaid = func(paidAt time.Time) {
		creditedAt := now.Add(-time.Millisecond)
		repo.wallet.BalanceUnits += repo.recharge.AmountCents
		repo.wallet.TotalRechargeUnits += repo.recharge.AmountCents
		repo.recharge.Status = rechargeStatusCredited
		repo.recharge.PaidAt = &paidAt
		repo.recharge.CreditedAt = &creditedAt
		repo.creditCount++
	}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605212200", now, finalizeSourceCallback)
	if appErr != nil {
		t.Fatalf("FinalizeOrderPaid stale snapshot error=%v", appErr)
	}
	if repo.recharge.Status != rechargeStatusCredited || repo.recharge.CreditedAt == nil {
		t.Fatalf("stale finalizer must not downgrade credited recharge, result=%#v recharge=%#v", result, repo.recharge)
	}
	if repo.wallet.BalanceUnits != 1000 || repo.creditCount != 1 {
		t.Fatalf("stale finalizer must not double credit wallet=%#v creditCount=%d", repo.wallet, repo.creditCount)
	}
	if !result.RechargeCredited {
		t.Fatalf("stale finalizer should observe credited recharge, result=%#v", result)
	}
}

func TestFinalizeOrderPaidDoesNotCreditRechargeClosedAfterStaleSnapshot(t *testing.T) {
	repo := newFakeRechargeRepo()
	now := fixedRechargeNow()
	repo.wallet = &Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo}
	repo.order = &Order{ID: 1, OrderNo: "PAY20260521100000000000", ConfigID: 1, ConfigCode: "alipay_default", Provider: providerAlipay, AmountCents: 1000, Status: orderStatusPaying, IsDel: enum.CommonNo}
	repo.recharge = &Recharge{ID: 1, RechargeNo: "RCG20260521100000000000", UserID: 7, PaymentOrderID: repo.order.ID, Status: rechargeStatusPaying, AmountCents: 1000, IsDel: enum.CommonNo}
	repo.beforeUpdateRechargePaid = func(paidAt time.Time) {
		repo.recharge.Status = rechargeStatusClosed
	}
	service := newRechargeService(repo, &fakeOrderGateway{})

	result, appErr := service.FinalizeOrderPaid(context.Background(), repo.order.ID, "202605212200", now, finalizeSourceCallback)
	if appErr == nil {
		t.Fatalf("expected closed recharge conflict, got result=%#v", result)
	}
	if repo.recharge.Status != rechargeStatusClosed || repo.creditCount != 0 || repo.wallet.BalanceUnits != 0 {
		t.Fatalf("stale finalizer must not reopen or credit closed recharge, recharge=%#v wallet=%#v creditCount=%d", repo.recharge, repo.wallet, repo.creditCount)
	}
}

func TestCloseOrderAndLinkedRechargeDoesNotOverwritePaidOrder(t *testing.T) {
	paidAt := fixedOrderNow()
	repo := newFakeOrderRepoWithOrder(orderStatusPaid)
	repo.order.PaidAt = &paidAt
	repo.recharge = &Recharge{ID: 10, RechargeNo: "RCG20260521100000000000", UserID: 7, PaymentOrderID: repo.order.ID, Status: rechargeStatusPaid, AmountCents: repo.order.AmountCents, PaidAt: &paidAt, IsDel: enum.CommonNo}

	appErr := closeOrderAndLinkedRecharge(context.Background(), repo, repo.order.ID, fixedOrderNow().Add(time.Minute))
	if appErr != nil {
		t.Fatalf("closeOrderAndLinkedRecharge error=%v", appErr)
	}
	if repo.order.Status != orderStatusPaid || repo.order.ClosedAt != nil {
		t.Fatalf("close helper must not overwrite paid order, order=%#v", repo.order)
	}
	if repo.recharge.Status != rechargeStatusPaid {
		t.Fatalf("close helper must not close paid recharge, recharge=%#v", repo.recharge)
	}
}
