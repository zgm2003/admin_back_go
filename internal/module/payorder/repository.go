package payorder

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error)
	Detail(ctx context.Context, id int64) (*DetailRow, error)
	Items(ctx context.Context, orderID int64) ([]OrderItem, error)
	Get(ctx context.Context, id int64) (*Order, error)
	UpdateRemark(ctx context.Context, id int64, remark string) (int64, error)
	CloseOrder(ctx context.Context, id int64, currentStatus int, reason string, now time.Time) (int64, error)
	CloseLastActiveTransaction(ctx context.Context, orderID int64, now time.Time) (int64, error)
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.baseListQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			o.id,
			o.order_no,
			o.user_id,
			u.username AS user_name,
			u.email AS user_email,
			o.order_type,
			o.title,
			o.total_amount,
			o.discount_amount,
			o.pay_amount,
			o.pay_status,
			o.biz_status,
			o.admin_remark,
			o.pay_time,
			o.created_at
		`).
		Order("o.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Table("orders AS o").
		Where("o.is_del = ?", enum.CommonNo)
	if strings.TrimSpace(query.OrderNo) != "" {
		db = db.Where("o.order_no = ?", strings.TrimSpace(query.OrderNo))
	}
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("o.user_id = ?", *query.UserID)
	}

	var rows []struct {
		PayStatus int
		Num       int64
	}
	if err := db.Select("o.pay_status, COUNT(*) AS num").Group("o.pay_status").Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[int]int64, len(rows))
	for _, row := range rows {
		result[row.PayStatus] = row.Num
	}
	return result, nil
}

func (r *GormRepository) Detail(ctx context.Context, id int64) (*DetailRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row DetailRow
	err := r.db.WithContext(ctx).
		Table("orders AS o").
		Joins("LEFT JOIN users AS u ON u.id = o.user_id AND u.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN pay_channel AS pc ON pc.id = o.channel_id AND pc.is_del = ?", enum.CommonNo).
		Where("o.id = ?", id).
		Where("o.is_del = ?", enum.CommonNo).
		Select(`
			o.id,
			o.order_no,
			o.user_id,
			u.username AS user_name,
			u.email AS user_email,
			o.order_type,
			o.biz_type,
			o.biz_id,
			o.title,
			o.total_amount,
			o.discount_amount,
			o.pay_amount,
			o.pay_status,
			o.biz_status,
			o.success_transaction_id,
			o.channel_id,
			o.pay_method,
			o.pay_time,
			o.expire_time,
			o.close_time,
			o.biz_done_at,
			o.close_reason,
			CAST(o.extra AS CHAR) AS extra,
			o.admin_remark,
			o.created_at,
			pc.name AS channel_name,
			pc.channel AS channel
		`).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Items(ctx context.Context, orderID int64) ([]OrderItem, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []OrderItem
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Where("is_del = ?", enum.CommonNo).
		Order("id asc").
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Order
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateRemark(ctx context.Context, id int64, remark string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).
		Model(&Order{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"admin_remark": remark, "updated_at": time.Now()})
	return result.RowsAffected, result.Error
}

func (r *GormRepository) CloseOrder(ctx context.Context, id int64, currentStatus int, reason string, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).
		Model(&Order{}).
		Where("id = ?", id).
		Where("pay_status = ?", currentStatus).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"pay_status": enum.PayStatusClosed, "close_time": now, "close_reason": reason, "updated_at": now})
	return result.RowsAffected, result.Error
}

func (r *GormRepository) CloseLastActiveTransaction(ctx context.Context, orderID int64, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	var row struct{ ID int64 }
	result := r.db.WithContext(ctx).
		Table("pay_transactions").
		Select("id").
		Where("order_id = ?", orderID).
		Where("status IN ?", []int{enum.PayTxnCreated, enum.PayTxnWaiting}).
		Where("is_del = ?", enum.CommonNo).
		Order("attempt_no desc, id desc").
		Limit(1).
		Scan(&row)
	if result.Error != nil || result.RowsAffected == 0 || row.ID <= 0 {
		return 0, result.Error
	}
	result = r.db.WithContext(ctx).
		Table("pay_transactions").
		Where("id = ?", row.ID).
		Where("status IN ?", []int{enum.PayTxnCreated, enum.PayTxnWaiting}).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"status": enum.PayTxnClosed, "closed_at": now, "updated_at": now})
	return result.RowsAffected, result.Error
}

func (r *GormRepository) baseListQuery(ctx context.Context, query ListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("orders AS o").
		Joins("LEFT JOIN users AS u ON u.id = o.user_id AND u.is_del = ?", enum.CommonNo).
		Where("o.is_del = ?", enum.CommonNo)

	if query.OrderType != nil {
		db = db.Where("o.order_type = ?", *query.OrderType)
	}
	if query.PayStatus != nil {
		db = db.Where("o.pay_status = ?", *query.PayStatus)
	}
	if strings.TrimSpace(query.OrderNo) != "" {
		db = db.Where("o.order_no = ?", strings.TrimSpace(query.OrderNo))
	}
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("o.user_id = ?", *query.UserID)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("o.created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("o.created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}
