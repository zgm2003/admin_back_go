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
func (r *fakeConfigRepo) UpdateRechargePaid(ctx context.Context, id int64, paidAt time.Time) error {
	return nil
}
func (r *fakeConfigRepo) UpdateRechargeClosed(ctx context.Context, id int64) error { return nil }
func (r *fakeConfigRepo) CreditRecharge(ctx context.Context, rechargeID int64, paidAt time.Time, now time.Time) (*Wallet, *Recharge, error) {
	return &Wallet{ID: 1, UserID: 1, IsDel: 2}, &Recharge{ID: rechargeID, Status: rechargeStatusCredited, PaidAt: &paidAt, CreditedAt: &now}, nil
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
func (r *fakeOrderRepo) GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeWithOrder, error) {
	return nil, nil
}
func (r *fakeOrderRepo) CreateRechargeWithOrder(ctx context.Context, recharge Recharge, order Order) (RechargeWithOrder, error) {
	return RechargeWithOrder{}, nil
}
func (r *fakeOrderRepo) UpdateRechargePaying(ctx context.Context, id int64) error { return nil }
func (r *fakeOrderRepo) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	return nil
}
func (r *fakeOrderRepo) UpdateRechargePaid(ctx context.Context, id int64, paidAt time.Time) error {
	return nil
}
func (r *fakeOrderRepo) UpdateRechargeClosed(ctx context.Context, id int64) error { return nil }
func (r *fakeOrderRepo) CreditRecharge(ctx context.Context, rechargeID int64, paidAt time.Time, now time.Time) (*Wallet, *Recharge, error) {
	return &Wallet{ID: 1, UserID: 1, IsDel: 2}, &Recharge{ID: rechargeID, Status: rechargeStatusCredited, PaidAt: &paidAt, CreditedAt: &now}, nil
}
func (r *fakeOrderRepo) FirstEnabledConfigForPay(ctx context.Context, provider string, payMethod string) (*Config, error) {
	return r.config, nil
}
