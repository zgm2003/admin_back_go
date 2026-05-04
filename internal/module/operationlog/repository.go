package operationlog

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("operation log repository not configured")

type Repository interface {
	Create(ctx context.Context, row Log) error
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Delete(ctx context.Context, ids []int64) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) Repository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) Create(ctx context.Context, row Log) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).
		Table("operation_logs AS o").
		Joins("LEFT JOIN users AS u ON u.id = o.user_id").
		Where("o.is_del = ?", CommonNo)

	if query.UserID > 0 {
		db = db.Where("o.user_id = ?", query.UserID)
	}
	if query.Action != "" {
		db = db.Where("o.action LIKE ?", strings.TrimSpace(query.Action)+"%")
	}
	if len(query.DateRange) == 2 {
		start := strings.TrimSpace(query.DateRange[0])
		end := strings.TrimSpace(query.DateRange[1])
		if start != "" && end != "" {
			db = db.Where("o.created_at BETWEEN ? AND ?", start+" 00:00:00", end+" 23:59:59")
		}
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			o.id,
			o.user_id,
			COALESCE(u.username, '') AS user_name,
			COALESCE(u.email, '') AS user_email,
			o.action,
			o.request_data,
			o.response_data,
			o.is_success,
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

func (r *GormRepository) Delete(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Log{}).
		Where("id IN ?", ids).
		Where("is_del = ?", CommonNo).
		Update("is_del", CommonYes).Error
}
