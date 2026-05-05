package paychannel

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Channel, int64, error)
	Get(ctx context.Context, id int64) (*Channel, error)
	ExistsUnique(ctx context.Context, channel int, mchID string, appID string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Channel) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id int64, status int) error
	Delete(ctx context.Context, id int64) error
	Referenced(ctx context.Context, id int64) (bool, error)
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Channel, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Channel{}).Where("is_del = ?", enum.CommonNo)
	name := strings.TrimSpace(query.Name)
	if name != "" {
		db = db.Where("name LIKE ?", name+"%")
	}
	if query.Channel != nil {
		db = db.Where("channel = ?", *query.Channel)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Channel
	if err := db.Order("sort asc, id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Channel, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Channel
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

func (r *GormRepository) ExistsUnique(ctx context.Context, channel int, mchID string, appID string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Model(&Channel{}).
		Where("channel = ?", channel).
		Where("mch_id = ?", mchID).
		Where("app_id = ?", appID).
		Where("is_del = ?", enum.CommonNo)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Channel) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Channel{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Channel{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Channel{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) Referenced(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var orderCount int64
	if err := r.db.WithContext(ctx).Table("orders").Where("channel_id = ?", id).Count(&orderCount).Error; err != nil {
		return false, err
	}
	if orderCount > 0 {
		return true, nil
	}
	var transactionCount int64
	if err := r.db.WithContext(ctx).Table("pay_transactions").Where("channel_id = ?", id).Count(&transactionCount).Error; err != nil {
		return false, err
	}
	return transactionCount > 0, nil
}
