package payment

import (
	"context"
	"errors"
	"strings"
	"time"

	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/money"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	duplicateKeyWalletTransactionNo = "uk_wallet_transaction_no"
	maxWalletTransactionNoAttempts  = 3
)

func (r *GormRepository) ListRechargePackages(ctx context.Context) ([]RechargePackage, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []RechargePackage
	err := r.db.WithContext(ctx).
		Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).
		Order("sort asc, id asc").
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetRechargePackageByCode(ctx context.Context, code string) (*RechargePackage, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row RechargePackage
	err := r.db.WithContext(ctx).
		Where("code = ? AND is_del = ? AND status = ?", strings.TrimSpace(code), enum.CommonNo, enum.CommonYes).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) FirstEnabledConfigForPay(ctx context.Context, provider string, payMethod string) (*Config, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Config
	err := r.db.WithContext(ctx).
		Where("provider = ? AND status = ? AND is_del = ?", strings.TrimSpace(provider), enum.CommonYes, enum.CommonNo).
		Order("sort asc, id asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	method := strings.TrimSpace(payMethod)
	for idx := range rows {
		if methodEnabled(rows[idx].EnabledMethodsJSON, method) {
			return &rows[idx], nil
		}
	}
	return nil, nil
}

func (r *GormRepository) ListRecharges(ctx context.Context, query RechargeListQuery) ([]RechargeWithOrder, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := rechargeJoinQuery(r.db.WithContext(ctx)).Where("r.user_id = ? AND r.is_del = ?", query.UserID, enum.CommonNo)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := keyword + "%"
		db = db.Where("(r.recharge_no LIKE ? OR po.order_no LIKE ? OR r.package_name LIKE ?)", like, like, like)
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		db = db.Where("r.status = ?", status)
	}
	if start := strings.TrimSpace(query.DateStart); start != "" {
		db = db.Where("r.created_at >= ?", start)
	}
	if end := strings.TrimSpace(query.DateEnd); end != "" {
		db = db.Where("r.created_at <= ?", end)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []RechargeWithOrder
	err := db.Order("r.id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) ListRecentRecharges(ctx context.Context, userID int64, limit int) ([]RechargeWithOrder, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	var rows []RechargeWithOrder
	err := rechargeJoinQuery(r.db.WithContext(ctx)).
		Where("r.user_id = ? AND r.is_del = ?", userID, enum.CommonNo).
		Order("r.id desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) ListUncreditedPaidRecharges(ctx context.Context, limit int) ([]RechargeWithOrder, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit = normalizePaymentJobLimit(limit)
	var rows []RechargeWithOrder
	err := rechargeJoinQuery(r.db.WithContext(ctx)).
		Where("po.status = ? AND r.status IN ? AND r.credited_at IS NULL AND r.is_del = ?", orderStatusPaid, []string{rechargeStatusPending, rechargeStatusPaying, rechargeStatusPaid}, enum.CommonNo).
		Order("r.id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeWithOrder, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row RechargeWithOrder
	err := rechargeJoinQuery(r.db.WithContext(ctx)).
		Where("r.id = ? AND r.user_id = ? AND r.is_del = ?", id, userID, enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetRechargeByOrderID(ctx context.Context, orderID int64) (*Recharge, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Recharge
	err := r.db.WithContext(ctx).
		Where("payment_order_id = ? AND is_del = ?", orderID, enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateRechargeWithOrder(ctx context.Context, recharge Recharge, order Order) (RechargeWithOrder, error) {
	if r == nil || r.db == nil {
		return RechargeWithOrder{}, ErrRepositoryNotConfigured
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order.IsDel = enum.CommonNo
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		recharge.PaymentOrderID = order.ID
		recharge.IsDel = enum.CommonNo
		if err := tx.Create(&recharge).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return RechargeWithOrder{}, err
	}
	row, err := r.GetRecharge(ctx, recharge.UserID, recharge.ID)
	if err != nil {
		return RechargeWithOrder{}, err
	}
	if row == nil {
		return RechargeWithOrder{}, gorm.ErrRecordNotFound
	}
	return *row, nil
}

func (r *GormRepository) UpdateRechargePaying(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{
		"status":         rechargeStatusPaying,
		"failure_reason": "",
	}).Error
}

func (r *GormRepository) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{
		"status":         rechargeStatusFailed,
		"failure_reason": trimMax(reason, 255),
	}).Error
}

func (r *GormRepository) UpdateRechargePaid(ctx context.Context, id int64, paidAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Recharge{}).
		Where("id = ? AND is_del = ? AND status IN ? AND credited_at IS NULL", id, enum.CommonNo, rechargePaidCASStatuses).
		Updates(map[string]any{
			"status":         rechargeStatusPaid,
			"paid_at":        paidAt,
			"failure_reason": "",
		}).Error
}

func (r *GormRepository) UpdateRechargeClosed(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, rechargeClosedCASStatuses).Update("status", rechargeStatusClosed).Error
}

func (r *GormRepository) CreditRecharge(ctx context.Context, rechargeID int64, paidAt time.Time, now time.Time) (*Wallet, *Recharge, error) {
	if r == nil || r.db == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	var creditedWallet Wallet
	var creditedRecharge Recharge
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var recharge Recharge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND is_del = ?", rechargeID, enum.CommonNo).
			First(&recharge).Error; err != nil {
			return err
		}
		units, err := money.CentsToUnits(recharge.AmountCents)
		if err != nil {
			return err
		}
		if recharge.Status == rechargeStatusClosed || recharge.Status == rechargeStatusFailed {
			return ErrPaymentStateChanged
		}
		if recharge.CreditedAt != nil || recharge.Status == rechargeStatusCredited {
			if recharge.Status != rechargeStatusCredited {
				updates := map[string]any{
					"status":         rechargeStatusCredited,
					"failure_reason": "",
				}
				if recharge.PaidAt == nil {
					updates["paid_at"] = paidAt
					recharge.PaidAt = &paidAt
				}
				if recharge.CreditedAt == nil {
					updates["credited_at"] = now
					recharge.CreditedAt = &now
				}
				if err := tx.Model(&Recharge{}).Where("id = ? AND is_del = ?", recharge.ID, enum.CommonNo).Updates(updates).Error; err != nil {
					return err
				}
				recharge.Status = rechargeStatusCredited
				recharge.FailureReason = ""
			}
			creditedRecharge = recharge
			return nil
		}
		if r.walletParticipant == nil {
			return ErrRepositoryNotConfigured
		}
		wallet, transaction, err := r.walletParticipant.CreditRechargeInTx(ctx, tx, walletmodule.CreditRechargeInput{UserID: recharge.UserID, RechargeID: recharge.ID, AmountUnits: units, Remark: "支付宝充值"})
		if err != nil {
			return err
		}
		if wallet != nil {
			creditedWallet = Wallet{ID: wallet.ID, UserID: wallet.UserID, BalanceUnits: wallet.BalanceUnits, TotalRechargeUnits: wallet.TotalRechargeUnits, TotalConsumeUnits: wallet.TotalConsumeUnits, HeldUnits: wallet.HeldUnits, IsDel: wallet.IsDel, CreatedAt: wallet.CreatedAt, UpdatedAt: wallet.UpdatedAt}
		}
		if transaction == nil {
			return ErrPaymentStateChanged
		}
		if err := tx.Model(&Recharge{}).Where("id = ? AND is_del = ?", recharge.ID, enum.CommonNo).Updates(map[string]any{
			"status": rechargeStatusCredited, "paid_at": paidAt, "credited_at": now, "failure_reason": "",
		}).Error; err != nil {
			return err
		}
		recharge.Status = rechargeStatusCredited
		recharge.PaidAt = &paidAt
		recharge.CreditedAt = &now
		creditedRecharge = recharge
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &creditedWallet, &creditedRecharge, nil
}

func createWalletTransactionWithNumberRetry(tx *gorm.DB, row *WalletTransaction, now time.Time) error {
	var err error
	for attempt := 0; attempt < maxWalletTransactionNoAttempts; attempt++ {
		if attempt > 0 {
			row.TransactionNo = newWalletTransactionNo(now)
		}
		err = tx.Create(row).Error
		if err == nil {
			return nil
		}
		if !isDuplicateKeyFor(err, duplicateKeyWalletTransactionNo) {
			return err
		}
	}
	return err
}

func isDuplicateKeyFor(err error, key string) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return strings.Contains(mysqlErr.Message, key)
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") && strings.Contains(msg, key)
}

func rechargeJoinQuery(db *gorm.DB) *gorm.DB {
	return db.Table("payment_recharges AS r").
		Select(`r.*, po.order_no AS payment_order_no, po.pay_url AS pay_url, po.status AS order_status, po.alipay_trade_no AS alipay_trade_no, po.paid_at AS order_paid_at`).
		Joins("JOIN payment_orders AS po ON po.id = r.payment_order_id AND po.is_del = ?", enum.CommonNo)
}
