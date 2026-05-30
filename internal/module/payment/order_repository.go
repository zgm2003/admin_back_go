package payment

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

func (r *GormRepository) ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Model(&Order{}).Where("is_del = ?", enum.CommonNo)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := keyword + "%"
		db = db.Where("order_no LIKE ? OR subject LIKE ?", like, like)
	}
	if configCode := strings.TrimSpace(query.ConfigCode); configCode != "" {
		db = db.Where("config_code = ?", configCode)
	}
	if provider := strings.TrimSpace(query.Provider); provider != "" {
		db = db.Where("provider = ?", provider)
	}
	if payMethod := strings.TrimSpace(query.PayMethod); payMethod != "" {
		db = db.Where("pay_method = ?", payMethod)
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		db = db.Where("status = ?", status)
	}
	if start := strings.TrimSpace(query.DateStart); start != "" {
		db = db.Where("created_at >= ?", start)
	}
	if end := strings.TrimSpace(query.DateEnd); end != "" {
		db = db.Where("created_at <= ?", end)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Order
	err := db.Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetOrder(ctx context.Context, id int64) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Order
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ?", id, enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateOrder(ctx context.Context, order Order) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	order.IsDel = enum.CommonNo
	if err := r.db.WithContext(ctx).Create(&order).Error; err != nil {
		return 0, err
	}
	return order.ID, nil
}

func (r *GormRepository) UpdateOrderPaying(ctx context.Context, id int64, payURL string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, orderPayingCASStatuses).Updates(map[string]any{
		"status":         orderStatusPaying,
		"pay_url":        strings.TrimSpace(payURL),
		"failure_reason": "",
	})
	return ensurePaymentRowsChanged(tx)
}

func (r *GormRepository) UpdateOrderFailed(ctx context.Context, id int64, reason string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, orderFailedCASStatuses).Updates(map[string]any{
		"status":         orderStatusFailed,
		"failure_reason": trimMax(reason, 255),
	})
	return ensurePaymentRowsChanged(tx)
}

func (r *GormRepository) UpdateOrderPaid(ctx context.Context, id int64, tradeNo string, paidAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, orderPaidCASStatuses).Updates(map[string]any{
		"status":          orderStatusPaid,
		"alipay_trade_no": strings.TrimSpace(tradeNo),
		"paid_at":         paidAt,
		"failure_reason":  "",
	})
	return ensurePaymentRowsChanged(tx)
}

func (r *GormRepository) UpdateOrderClosed(ctx context.Context, id int64, closedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND is_del = ? AND status IN ?", id, enum.CommonNo, orderClosedCASStatuses).Updates(map[string]any{
		"status":    orderStatusClosed,
		"closed_at": closedAt,
	})
	return ensurePaymentRowsChanged(tx)
}

func (r *GormRepository) GetOrderByNo(ctx context.Context, orderNo string) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Order
	err := r.db.WithContext(ctx).Where("order_no = ? AND is_del = ?", strings.TrimSpace(orderNo), enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ListPendingPayingOrders(ctx context.Context, cutoff time.Time, limit int) ([]Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = defaultPaymentJobLimit
	}
	var rows []Order
	err := r.db.WithContext(ctx).
		Where("provider = ? AND status = ? AND is_del = ? AND updated_at <= ?", providerAlipay, orderStatusPaying, enum.CommonNo, cutoff).
		Order("id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) ListExpiredOpenOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = defaultPaymentJobLimit
	}
	var rows []Order
	err := r.db.WithContext(ctx).
		Where("provider = ? AND status IN ? AND is_del = ? AND expired_at < ?", providerAlipay, []string{orderStatusPending, orderStatusPaying}, enum.CommonNo, now).
		Order("expired_at asc, id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) ListEnabledOrderConfigOptions(ctx context.Context) ([]Config, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Config
	err := r.db.WithContext(ctx).Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).Order("sort asc, id asc").Find(&rows).Error
	return rows, err
}

func trimMax(value string, max int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= max {
		return value
	}
	return string([]rune(value)[:max])
}

func ensurePaymentRowsChanged(tx *gorm.DB) error {
	if tx == nil {
		return ErrRepositoryNotConfigured
	}
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return ErrPaymentStateChanged
	}
	return nil
}
