package payment

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	return r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{
		"status":         rechargeStatusPaid,
		"paid_at":        paidAt,
		"failure_reason": "",
	}).Error
}

func (r *GormRepository) UpdateRechargeClosed(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("status", rechargeStatusClosed).Error
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
		wallet, err := lockWalletForUpdate(tx, recharge.UserID)
		if err != nil {
			return err
		}
		var existing int64
		if err := tx.Model(&WalletTransaction{}).
			Where("source_type = ? AND source_id = ? AND is_del = ?", walletSourceRecharge, recharge.ID, enum.CommonNo).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 || recharge.CreditedAt != nil || recharge.Status == rechargeStatusCredited {
			creditedWallet = *wallet
			creditedRecharge = recharge
			return nil
		}
		before := wallet.BalanceCents
		after := before + recharge.AmountCents
		txRow := WalletTransaction{
			TransactionNo:      newWalletTransactionNo(now),
			WalletID:           wallet.ID,
			UserID:             recharge.UserID,
			Direction:          walletDirectionIn,
			AmountCents:        recharge.AmountCents,
			BalanceBeforeCents: before,
			BalanceAfterCents:  after,
			SourceType:         walletSourceRecharge,
			SourceID:           recharge.ID,
			Remark:             "支付宝充值",
			IsDel:              enum.CommonNo,
		}
		if err := tx.Create(&txRow).Error; err != nil {
			return err
		}
		if err := tx.Model(&Wallet{}).Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).Updates(map[string]any{
			"balance_cents":        after,
			"total_recharge_cents": wallet.TotalRechargeCents + recharge.AmountCents,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Recharge{}).Where("id = ? AND is_del = ?", recharge.ID, enum.CommonNo).Updates(map[string]any{
			"status":         rechargeStatusCredited,
			"paid_at":        paidAt,
			"credited_at":    now,
			"failure_reason": "",
		}).Error; err != nil {
			return err
		}
		wallet.BalanceCents = after
		wallet.TotalRechargeCents += recharge.AmountCents
		recharge.Status = rechargeStatusCredited
		recharge.PaidAt = &paidAt
		recharge.CreditedAt = &now
		creditedWallet = *wallet
		creditedRecharge = recharge
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &creditedWallet, &creditedRecharge, nil
}

func rechargeJoinQuery(db *gorm.DB) *gorm.DB {
	return db.Table("payment_recharges AS r").
		Select(`r.*, po.order_no AS payment_order_no, po.pay_url AS pay_url, po.status AS order_status, po.alipay_trade_no AS alipay_trade_no, po.paid_at AS order_paid_at`).
		Joins("JOIN payment_orders AS po ON po.id = r.payment_order_id AND po.is_del = ?", enum.CommonNo)
}
