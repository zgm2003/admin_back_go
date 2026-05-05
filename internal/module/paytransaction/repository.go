package paytransaction

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Detail(ctx context.Context, id int64) (*DetailRow, error)
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
			pt.id,
			pt.transaction_no,
			pt.order_no,
			o.user_id,
			u.username AS user_name,
			u.email AS user_email,
			pt.attempt_no,
			pt.channel_id,
			pt.channel,
			pt.pay_method,
			pt.amount,
			pt.trade_no,
			pt.trade_status,
			pt.status,
			pt.paid_at,
			pt.created_at
		`).
		Order("pt.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
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
		Table("pay_transactions AS pt").
		Joins("LEFT JOIN orders AS o ON o.id = pt.order_id AND o.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN users AS u ON u.id = o.user_id AND u.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN pay_channel AS pc ON pc.id = pt.channel_id AND pc.is_del = ?", enum.CommonNo).
		Where("pt.id = ?", id).
		Where("pt.is_del = ?", enum.CommonNo).
		Select(`
			pt.id,
			pt.transaction_no,
			pt.order_id,
			pt.order_no,
			pt.attempt_no,
			pt.channel_id,
			pt.channel,
			pt.pay_method,
			pt.amount,
			pt.trade_no,
			pt.trade_status,
			pt.status,
			pt.paid_at,
			pt.closed_at,
			CAST(pt.channel_resp AS CHAR) AS channel_resp,
			CAST(pt.raw_notify AS CHAR) AS raw_notify,
			pt.created_at,
			pc.name AS pay_channel_name,
			o.user_id AS order_user_id,
			u.username AS order_user_name,
			u.email AS order_user_email,
			o.title AS order_title,
			o.pay_amount AS order_pay_amount,
			o.pay_status AS order_pay_status
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

func (r *GormRepository) baseListQuery(ctx context.Context, query ListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("pay_transactions AS pt").
		Joins("LEFT JOIN orders AS o ON o.id = pt.order_id AND o.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN users AS u ON u.id = o.user_id AND u.is_del = ?", enum.CommonNo).
		Where("pt.is_del = ?", enum.CommonNo)

	if query.OrderNo != "" {
		db = db.Where("pt.order_no = ?", query.OrderNo)
	}
	if query.TransactionNo != "" {
		db = db.Where("pt.transaction_no = ?", query.TransactionNo)
	}
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("o.user_id = ?", *query.UserID)
	}
	if query.Channel != nil {
		db = db.Where("pt.channel = ?", *query.Channel)
	}
	if query.Status != nil {
		db = db.Where("pt.status = ?", *query.Status)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("pt.created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("pt.created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}
