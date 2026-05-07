package paynotifylog

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
			id,
			channel,
			notify_type,
			transaction_no,
			trade_no,
			process_status,
			process_msg,
			ip,
			created_at
		`).
		Order("id desc").
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
		Table("pay_notify_logs").
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Select(`
			id,
			channel,
			notify_type,
			transaction_no,
			trade_no,
			process_status,
			process_msg,
			CAST(headers AS CHAR) AS headers,
			CAST(raw_data AS CHAR) AS raw_data,
			ip,
			created_at,
			updated_at
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
		Table("pay_notify_logs").
		Where("is_del = ?", enum.CommonNo)

	if query.TransactionNo != "" {
		db = db.Where("transaction_no = ?", query.TransactionNo)
	}
	if query.Channel != nil {
		db = db.Where("channel = ?", *query.Channel)
	}
	if query.NotifyType != nil {
		db = db.Where("notify_type = ?", *query.NotifyType)
	}
	if query.ProcessStatus != nil {
		db = db.Where("process_status = ?", *query.ProcessStatus)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}
