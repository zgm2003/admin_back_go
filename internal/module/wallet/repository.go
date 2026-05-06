package wallet

import (
	"context"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Transactions(ctx context.Context, query TransactionListQuery) ([]TransactionRow, int64, error)
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
	db := r.baseWalletQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			w.id,
			w.user_id,
			u.username AS user_name,
			u.email AS user_email,
			w.balance,
			w.frozen,
			w.total_recharge,
			w.total_consume,
			w.created_at
		`).
		Order("w.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Transactions(ctx context.Context, query TransactionListQuery) ([]TransactionRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.baseTransactionQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []TransactionRow
	err := db.Select(`
			wt.id,
			wt.user_id,
			u.username AS user_name,
			u.email AS user_email,
			wt.biz_action_no,
			wt.type,
			wt.available_delta,
			wt.frozen_delta,
			wt.balance_before,
			wt.balance_after,
			wt.order_no,
			wt.title,
			wt.remark,
			wt.created_at
		`).
		Order("wt.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) baseWalletQuery(ctx context.Context, query ListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("user_wallets AS w").
		Joins("LEFT JOIN users AS u ON u.id = w.user_id AND u.is_del = ?", enum.CommonNo).
		Where("w.is_del = ?", enum.CommonNo)
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("w.user_id = ?", *query.UserID)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("w.created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("w.created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}

func (r *GormRepository) baseTransactionQuery(ctx context.Context, query TransactionListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("wallet_transactions AS wt").
		Joins("LEFT JOIN users AS u ON u.id = wt.user_id AND u.is_del = ?", enum.CommonNo).
		Where("wt.is_del = ?", enum.CommonNo)
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("wt.user_id = ?", *query.UserID)
	}
	if query.Type != nil {
		db = db.Where("wt.type = ?", *query.Type)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("wt.created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("wt.created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}
