package payment

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/money"

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
	result := r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, rechargePayingCASStatuses).Updates(map[string]any{
		"status":         rechargeStatusPaying,
		"failure_reason": "",
	})
	return rechargeUpdateResult(result)
}

func (r *GormRepository) UpdateRechargeFailed(ctx context.Context, id int64, reason string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, rechargeFailedCASStatuses).Updates(map[string]any{
		"status":         rechargeStatusFailed,
		"failure_reason": trimMax(reason, 255),
	})
	return rechargeUpdateResult(result)
}

func (r *GormRepository) UpdateRechargeClosed(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).Model(&Recharge{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, rechargeClosedCASStatuses).Update("status", rechargeStatusClosed)
	return rechargeUpdateResult(result)
}

func rechargeUpdateResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPaymentStateChanged
	}
	return nil
}

func (r *GormRepository) FinalizePaidOrder(ctx context.Context, orderID int64, tradeNo string, paidAt time.Time, now time.Time) (*PaidOrderFinalization, error) {
	if r == nil || r.db == nil || r.walletParticipant == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if orderID <= 0 || paidAt.IsZero() || now.IsZero() {
		return nil, ErrPaymentStateChanged
	}
	var fact PaidOrderFinalization
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentOrderNotFound
			}
			return err
		}
		orderAlreadyPaid, err := validateOrderForPaidFinalization(&order, orderID, tradeNo)
		if err != nil {
			return err
		}

		var row Recharge
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("payment_order_id = ?", order.ID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !orderAlreadyPaid {
				if err := markOrderPaidInTx(tx, &order, tradeNo, paidAt); err != nil {
					return err
				}
			}
			fact = PaidOrderFinalization{Order: &order, OrderPaid: !orderAlreadyPaid, OrderAlreadyPaid: orderAlreadyPaid, RawOrder: true}
			return nil
		}
		if err != nil {
			return err
		}
		rechargeAlreadyCredited, err := validateRechargeForPaidFinalization(&row, &order, orderAlreadyPaid)
		if err != nil {
			return err
		}
		units, err := money.CentsToUnits(row.AmountCents)
		if err != nil {
			return err
		}
		if !orderAlreadyPaid {
			if err := markOrderPaidInTx(tx, &order, tradeNo, paidAt); err != nil {
				return err
			}
		}
		creditInput := walletmodule.CreditRechargeInput{UserID: row.UserID, RechargeID: row.ID, AmountUnits: units, Remark: "支付宝充值"}
		var creditFact *walletmodule.RechargeCreditFact
		if rechargeAlreadyCredited {
			creditFact, err = r.walletParticipant.FindRechargeCreditInTx(ctx, tx, creditInput)
		} else {
			creditFact, err = r.walletParticipant.CreditRechargeInTx(ctx, tx, creditInput)
		}
		if err != nil {
			return err
		}
		if err := validateRechargeCreditParticipantFact(creditFact, &row, units, rechargeAlreadyCredited); err != nil {
			return err
		}
		if !rechargeAlreadyCredited {
			settledAt := paidAt
			if order.PaidAt != nil {
				settledAt = *order.PaidAt
			}
			if err := markRechargeCreditedInTx(tx, &row, settledAt, now); err != nil {
				return err
			}
		}
		fact = PaidOrderFinalization{
			Order:                   &order,
			Recharge:                &row,
			Wallet:                  creditFact.Wallet,
			OrderPaid:               !orderAlreadyPaid,
			OrderAlreadyPaid:        orderAlreadyPaid,
			RechargeCredited:        !rechargeAlreadyCredited,
			RechargeAlreadyCredited: rechargeAlreadyCredited,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &fact, nil
}

func validateOrderForPaidFinalization(order *Order, orderID int64, tradeNo string) (bool, error) {
	if order == nil || order.ID <= 0 || order.ID != orderID || order.IsDel != enum.CommonNo || order.AmountCents <= 0 {
		return false, ErrPaymentStateChanged
	}
	switch order.Status {
	case orderStatusPending, orderStatusPaying:
		if order.PaidAt != nil || order.ClosedAt != nil {
			return false, ErrPaymentStateChanged
		}
		return false, nil
	case orderStatusPaid:
		if order.PaidAt == nil || order.ClosedAt != nil {
			return false, ErrPaymentStateChanged
		}
		incomingTradeNo := strings.TrimSpace(tradeNo)
		existingTradeNo := strings.TrimSpace(order.AlipayTradeNo)
		if incomingTradeNo != "" && existingTradeNo != "" && incomingTradeNo != existingTradeNo {
			return false, ErrPaymentStateChanged
		}
		return true, nil
	default:
		return false, ErrPaymentStateChanged
	}
}

func validateRechargeForPaidFinalization(recharge *Recharge, order *Order, orderAlreadyPaid bool) (bool, error) {
	if recharge == nil || order == nil || recharge.ID <= 0 || recharge.UserID <= 0 || recharge.PaymentOrderID != order.ID ||
		recharge.AmountCents <= 0 || recharge.AmountCents != order.AmountCents || recharge.IsDel != enum.CommonNo {
		return false, ErrPaymentStateChanged
	}
	switch recharge.Status {
	case rechargeStatusPending, rechargeStatusPaying:
		if recharge.PaidAt != nil || recharge.CreditedAt != nil {
			return false, ErrPaymentStateChanged
		}
		return false, nil
	case rechargeStatusPaid:
		if !orderAlreadyPaid || recharge.PaidAt == nil || recharge.CreditedAt != nil {
			return false, ErrPaymentStateChanged
		}
		return false, nil
	case rechargeStatusCredited:
		if !orderAlreadyPaid || recharge.PaidAt == nil || recharge.CreditedAt == nil {
			return false, ErrPaymentStateChanged
		}
		return true, nil
	default:
		return false, ErrPaymentStateChanged
	}
}

func markOrderPaidInTx(tx *gorm.DB, order *Order, tradeNo string, paidAt time.Time) error {
	resolvedTradeNo := resultTradeNoFromStrings(tradeNo, order.AlipayTradeNo)
	result := tx.Model(&Order{}).
		Where("id = ? AND is_del = ? AND status = ? AND paid_at IS NULL AND closed_at IS NULL", order.ID, enum.CommonNo, order.Status).
		Updates(map[string]any{"status": orderStatusPaid, "paid_at": paidAt, "alipay_trade_no": resolvedTradeNo})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPaymentStateChanged
	}
	order.Status = orderStatusPaid
	order.PaidAt = &paidAt
	order.AlipayTradeNo = resolvedTradeNo
	return nil
}

func markRechargeCreditedInTx(tx *gorm.DB, recharge *Recharge, paidAt, creditedAt time.Time) error {
	query := tx.Model(&Recharge{}).
		Where("id = ? AND payment_order_id = ? AND user_id = ? AND amount_cents = ? AND is_del = ? AND status = ?", recharge.ID, recharge.PaymentOrderID, recharge.UserID, recharge.AmountCents, enum.CommonNo, recharge.Status)
	if recharge.PaidAt == nil {
		query = query.Where("paid_at IS NULL AND credited_at IS NULL")
	} else {
		query = query.Where("paid_at = ? AND credited_at IS NULL", *recharge.PaidAt)
	}
	result := query.Updates(map[string]any{"status": rechargeStatusCredited, "paid_at": paidAt, "credited_at": creditedAt, "failure_reason": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPaymentStateChanged
	}
	recharge.Status = rechargeStatusCredited
	recharge.PaidAt = &paidAt
	recharge.CreditedAt = &creditedAt
	recharge.FailureReason = ""
	return nil
}

func validateRechargeCreditParticipantFact(fact *walletmodule.RechargeCreditFact, recharge *Recharge, units int64, replay bool) error {
	if fact == nil || recharge == nil {
		return ErrPaymentStateChanged
	}
	wallet := fact.Wallet
	transaction := fact.Transaction
	expectedDisposition := walletmodule.RechargeCreditCreated
	if replay {
		expectedDisposition = walletmodule.RechargeCreditReplayed
	}
	if fact.Disposition != expectedDisposition || wallet == nil || transaction == nil || wallet.ID <= 0 || wallet.UserID != recharge.UserID || wallet.IsDel != enum.CommonNo ||
		transaction.ID <= 0 || strings.TrimSpace(transaction.TransactionNo) == "" || transaction.WalletID != wallet.ID || transaction.UserID != recharge.UserID ||
		transaction.Direction != walletmodule.DirectionIn || transaction.AmountUnits != units || transaction.SourceType != walletmodule.SourceRecharge ||
		transaction.SourceID != recharge.ID || transaction.IsDel != enum.CommonNo || transaction.BalanceBeforeUnits < 0 ||
		transaction.BalanceBeforeUnits > math.MaxInt64-units || transaction.BalanceAfterUnits != transaction.BalanceBeforeUnits+units ||
		wallet.BalanceUnits < 0 || wallet.TotalRechargeUnits < units || wallet.HeldUnits < 0 || wallet.HeldUnits > wallet.BalanceUnits {
		return ErrPaymentStateChanged
	}
	return nil
}

func rechargeJoinQuery(db *gorm.DB) *gorm.DB {
	return db.Table("payment_recharges AS r").
		Select(`r.*, po.order_no AS payment_order_no, po.pay_url AS pay_url, po.status AS order_status, po.alipay_trade_no AS alipay_trade_no, po.paid_at AS order_paid_at`).
		Joins("JOIN payment_orders AS po ON po.id = r.payment_order_id AND po.is_del = ?", enum.CommonNo)
}
