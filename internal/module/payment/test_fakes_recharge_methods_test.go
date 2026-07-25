package payment

import (
	"context"
	"time"
)

func (r *fakeConfigRepo) ListRechargePackages(ctx context.Context) ([]RechargePackage, error) {
	return nil, nil
}
func (r *fakeConfigRepo) GetRechargePackageByCode(ctx context.Context, code string) (*RechargePackage, error) {
	return nil, nil
}
func (r *fakeConfigRepo) GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error) {
	return &Wallet{ID: 1, UserID: userID, IsDel: 2}, nil
}
func (r *fakeConfigRepo) GetWallet(ctx context.Context, userID int64) (*Wallet, error) {
	return &Wallet{ID: 1, UserID: userID, IsDel: 2}, nil
}
func (r *fakeConfigRepo) ListRecharges(ctx context.Context, query RechargeListQuery) ([]RechargeWithOrder, int64, error) {
	return nil, 0, nil
}
func (r *fakeConfigRepo) ListRecentRecharges(ctx context.Context, userID int64, limit int) ([]RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeConfigRepo) ListUncreditedPaidRecharges(ctx context.Context, limit int) ([]RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeConfigRepo) GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeConfigRepo) CreateRechargeWithOrder(ctx context.Context, recharge Recharge, order Order) (RechargeWithOrder, error) {
	return RechargeWithOrder{}, nil
}
func (r *fakeConfigRepo) UpdateRechargePaying(ctx context.Context, id int64) error { return nil }
func (r *fakeConfigRepo) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	return nil
}
func (r *fakeConfigRepo) UpdateRechargeClosed(ctx context.Context, id int64) error { return nil }
func (r *fakeConfigRepo) FinalizePaidOrder(context.Context, int64, string, time.Time, time.Time) (*PaidOrderFinalization, error) {
	return nil, ErrPaymentOrderNotFound
}
func (r *fakeConfigRepo) FirstEnabledConfigForPay(ctx context.Context, provider string, payMethod string) (*Config, error) {
	return r.config, nil
}

func (r *fakeOrderRepo) ListRechargePackages(ctx context.Context) ([]RechargePackage, error) {
	return nil, nil
}
func (r *fakeOrderRepo) GetRechargePackageByCode(ctx context.Context, code string) (*RechargePackage, error) {
	return nil, nil
}
func (r *fakeOrderRepo) GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error) {
	return &Wallet{ID: 1, UserID: userID, IsDel: 2}, nil
}
func (r *fakeOrderRepo) GetWallet(ctx context.Context, userID int64) (*Wallet, error) {
	return &Wallet{ID: 1, UserID: userID, IsDel: 2}, nil
}
func (r *fakeOrderRepo) ListRecharges(ctx context.Context, query RechargeListQuery) ([]RechargeWithOrder, int64, error) {
	return nil, 0, nil
}
func (r *fakeOrderRepo) ListRecentRecharges(ctx context.Context, userID int64, limit int) ([]RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeOrderRepo) ListUncreditedPaidRecharges(ctx context.Context, limit int) ([]RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeOrderRepo) GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeOrderRepo) CreateRechargeWithOrder(ctx context.Context, recharge Recharge, order Order) (RechargeWithOrder, error) {
	return RechargeWithOrder{}, nil
}
func (r *fakeOrderRepo) UpdateRechargePaying(ctx context.Context, id int64) error {
	if r.recharge == nil || r.recharge.ID != id || (r.recharge.Status != rechargeStatusPending && r.recharge.Status != rechargeStatusFailed) {
		return ErrPaymentStateChanged
	}
	r.recharge.Status = rechargeStatusPaying
	r.recharge.FailureReason = ""
	return nil
}
func (r *fakeOrderRepo) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	if r.recharge == nil || r.recharge.ID != id || (r.recharge.Status != rechargeStatusPending && r.recharge.Status != rechargeStatusFailed) {
		return ErrPaymentStateChanged
	}
	r.recharge.Status = rechargeStatusFailed
	r.recharge.FailureReason = reason
	return nil
}
func (r *fakeOrderRepo) UpdateRechargeClosed(ctx context.Context, id int64) error {
	if r.recharge != nil && r.recharge.ID == id {
		if !canCloseLinkedRecharge(r.recharge.Status) {
			return nil
		}
		r.recharge.Status = rechargeStatusClosed
	}
	return nil
}
func (r *fakeOrderRepo) FinalizePaidOrder(_ context.Context, id int64, _ string, paidAt time.Time, _ time.Time) (*PaidOrderFinalization, error) {
	if r.order == nil || r.order.ID != id {
		return nil, ErrPaymentOrderNotFound
	}
	if r.order.Status != orderStatusPending && r.order.Status != orderStatusPaying && r.order.Status != orderStatusPaid {
		return nil, ErrPaymentStateChanged
	}
	alreadyPaid := r.order.Status == orderStatusPaid
	r.order.Status, r.order.PaidAt = orderStatusPaid, &paidAt
	return &PaidOrderFinalization{Order: r.order, Recharge: r.recharge, OrderPaid: !alreadyPaid, OrderAlreadyPaid: alreadyPaid, RawOrder: r.recharge == nil}, nil
}
func (r *fakeOrderRepo) FirstEnabledConfigForPay(ctx context.Context, provider string, payMethod string) (*Config, error) {
	return r.config, nil
}

func (r *fakeConfigRepo) GetOrderByNo(ctx context.Context, orderNo string) (*Order, error) {
	return nil, nil
}
func (r *fakeConfigRepo) ListPendingPayingOrders(ctx context.Context, cutoff time.Time, limit int) ([]Order, error) {
	return nil, nil
}
func (r *fakeConfigRepo) ListExpiredOpenOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	return nil, nil
}
func (r *fakeConfigRepo) GetRechargeByOrderID(ctx context.Context, orderID int64) (*Recharge, error) {
	return nil, nil
}
func (r *fakeOrderRepo) GetRechargeByOrderID(ctx context.Context, orderID int64) (*Recharge, error) {
	if r.recharge != nil && r.recharge.PaymentOrderID == orderID {
		return r.recharge, nil
	}
	return nil, nil
}
