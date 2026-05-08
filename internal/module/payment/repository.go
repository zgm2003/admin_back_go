package payment

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error
	ListChannels(ctx context.Context, query ChannelListQuery) ([]Channel, int64, error)
	GetChannel(ctx context.Context, id int64) (*Channel, error)
	GetChannelConfig(ctx context.Context, channelID int64) (*ChannelConfig, error)
	CreateChannel(ctx context.Context, channel Channel, cfg ChannelConfig) (int64, error)
	UpdateChannel(ctx context.Context, id int64, fields map[string]any, cfgFields map[string]any) error
	ChangeChannelStatus(ctx context.Context, id int64, status int) error
	DeleteChannel(ctx context.Context, id int64) error
	FindEnabledChannel(ctx context.Context, id int64) (*Channel, *ChannelConfig, error)
	CreateOrder(ctx context.Context, order Order) (*Order, error)
	GetOrderByNo(ctx context.Context, orderNo string) (*Order, error)
	GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, error)
	GetOrderByID(ctx context.Context, id int64) (*Order, error)
	ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error)
	MarkOrderPaying(ctx context.Context, orderID int64, outTradeNo string, payURL string, returnURL string, now time.Time) error
	MarkOrderSucceeded(ctx context.Context, orderID int64, tradeNo string, paidAt time.Time) error
	MarkOrderClosed(ctx context.Context, orderID int64, now time.Time) error
	CreateEvent(ctx context.Context, event Event) error
	GetEventByID(ctx context.Context, id int64) (*Event, error)
	ListEvents(ctx context.Context, query EventListQuery) ([]Event, int64, error)
	ListExpiredOrders(ctx context.Context, now time.Time, limit int) ([]Order, error)
	ListPendingOrders(ctx context.Context, now time.Time, limit int) ([]Order, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx})
	})
}

func (r *GormRepository) ListChannels(ctx context.Context, query ChannelListQuery) ([]Channel, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Model(&Channel{}).Where("is_del = ?", enum.CommonNo)
	if name := strings.TrimSpace(query.Name); name != "" {
		db = db.Where("name LIKE ?", name+"%")
	}
	if provider := strings.TrimSpace(query.Provider); provider != "" {
		db = db.Where("provider = ?", provider)
	}
	if query.Status > 0 {
		db = db.Where("status = ?", query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Channel
	err := db.Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetChannel(ctx context.Context, id int64) (*Channel, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Channel
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ?", id, enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetChannelConfig(ctx context.Context, channelID int64) (*ChannelConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row ChannelConfig
	err := r.db.WithContext(ctx).Where("channel_id = ?", channelID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateChannel(ctx context.Context, channel Channel, cfg ChannelConfig) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&channel).Error; err != nil {
			return err
		}
		cfg.ChannelID = channel.ID
		return tx.Create(&cfg).Error
	})
	return channel.ID, err
}

func (r *GormRepository) UpdateChannel(ctx context.Context, id int64, fields map[string]any, cfgFields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			if err := tx.Model(&Channel{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(fields).Error; err != nil {
				return err
			}
		}
		if len(cfgFields) > 0 {
			if err := tx.Model(&ChannelConfig{}).Where("channel_id = ?", id).Updates(cfgFields).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) ChangeChannelStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.UpdateChannel(ctx, id, map[string]any{"status": status}, nil)
}

func (r *GormRepository) DeleteChannel(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.UpdateChannel(ctx, id, map[string]any{"is_del": enum.CommonYes}, nil)
}

func (r *GormRepository) FindEnabledChannel(ctx context.Context, id int64) (*Channel, *ChannelConfig, error) {
	if r == nil || r.db == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	channel, err := r.GetChannel(ctx, id)
	if err != nil || channel == nil {
		return channel, nil, err
	}
	if channel.Status != enum.CommonYes {
		return channel, nil, nil
	}
	cfg, err := r.GetChannelConfig(ctx, channel.ID)
	return channel, cfg, err
}

func (r *GormRepository) CreateOrder(ctx context.Context, order Order) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
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

func (r *GormRepository) GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Order
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ? AND is_del = ?", strings.TrimSpace(orderNo), enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetOrderByID(ctx context.Context, id int64) (*Order, error) {
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

func (r *GormRepository) ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Model(&Order{}).Where("is_del = ?", enum.CommonNo)
	if orderNo := strings.TrimSpace(query.OrderNo); orderNo != "" {
		db = db.Where("order_no LIKE ?", orderNo+"%")
	}
	if query.UserID > 0 {
		db = db.Where("user_id = ?", query.UserID)
	}
	if query.Status > 0 {
		db = db.Where("status = ?", query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Order
	err := db.Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) MarkOrderPaying(ctx context.Context, orderID int64, outTradeNo string, payURL string, returnURL string, now time.Time) error {
	trimmedOutTradeNo := strings.TrimSpace(outTradeNo)
	if trimmedOutTradeNo == "" {
		return ErrOutTradeNoRequired
	}
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND status IN ? AND is_del = ?", orderID, []int{enum.PaymentOrderPending, enum.PaymentOrderPaying}, enum.CommonNo).Updates(map[string]any{
		"status":       enum.PaymentOrderPaying,
		"out_trade_no": &trimmedOutTradeNo,
		"pay_url":      payURL,
		"return_url":   returnURL,
		"updated_at":   now,
	}).Error
}

func (r *GormRepository) MarkOrderSucceeded(ctx context.Context, orderID int64, tradeNo string, paidAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND status IN ? AND is_del = ?", orderID, []int{enum.PaymentOrderPending, enum.PaymentOrderPaying}, enum.CommonNo).Updates(map[string]any{
		"status":     enum.PaymentOrderSucceeded,
		"trade_no":   strings.TrimSpace(tradeNo),
		"paid_at":    paidAt,
		"updated_at": paidAt,
	}).Error
}

func (r *GormRepository) MarkOrderClosed(ctx context.Context, orderID int64, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND status IN ? AND is_del = ?", orderID, []int{enum.PaymentOrderPending, enum.PaymentOrderPaying}, enum.CommonNo).Updates(map[string]any{
		"status":     enum.PaymentOrderClosed,
		"closed_at":  now,
		"updated_at": now,
	}).Error
}

func (r *GormRepository) CreateEvent(ctx context.Context, event Event) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Create(&event).Error
}

func (r *GormRepository) GetEventByID(ctx context.Context, id int64) (*Event, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Event
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ListEvents(ctx context.Context, query EventListQuery) ([]Event, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Model(&Event{})
	if orderNo := strings.TrimSpace(query.OrderNo); orderNo != "" {
		db = db.Where("order_no LIKE ?", orderNo+"%")
	}
	if outTradeNo := strings.TrimSpace(query.OutTradeNo); outTradeNo != "" {
		db = db.Where("out_trade_no LIKE ?", outTradeNo+"%")
	}
	if eventType := strings.TrimSpace(query.EventType); eventType != "" {
		db = db.Where("event_type = ?", eventType)
	}
	if query.ProcessStatus > 0 {
		db = db.Where("process_status = ?", query.ProcessStatus)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Event
	err := db.Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) ListExpiredOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit = normalizeLimit(limit)
	var rows []Order
	err := r.db.WithContext(ctx).
		Where("status IN ? AND expired_at <= ? AND is_del = ?", []int{enum.PaymentOrderPending, enum.PaymentOrderPaying}, now, enum.CommonNo).
		Order("id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) ListPendingOrders(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit = normalizeLimit(limit)
	cutoff := now.Add(-5 * time.Minute)
	var rows []Order
	err := r.db.WithContext(ctx).
		Where("status = ? AND updated_at <= ? AND is_del = ?", enum.PaymentOrderPaying, cutoff, enum.CommonNo).
		Order("id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func normalizePage(currentPage int, pageSize int) (int, int, int) {
	if currentPage <= 0 {
		currentPage = 1
	}
	pageSize = normalizeLimit(pageSize)
	return currentPage, pageSize, (currentPage - 1) * pageSize
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultPageSize
	}
	if limit > maxPageSize {
		return maxPageSize
	}
	return limit
}
