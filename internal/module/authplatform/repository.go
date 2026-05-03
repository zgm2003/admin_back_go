package authplatform

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	FindActiveByCode(ctx context.Context, code string) (*Platform, error)
}

type ManagementRepository interface {
	Repository
	List(ctx context.Context, query ListQuery) ([]Platform, int64, error)
	Get(ctx context.Context, id int64) (*Platform, error)
	PlatformsByIDs(ctx context.Context, ids []int64) (map[int64]Platform, error)
	ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Platform) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	Delete(ctx context.Context, ids []int64) error
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

func (r *GormRepository) FindActiveByCode(ctx context.Context, code string) (*Platform, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var platform Platform
	err := r.db.WithContext(ctx).
		Where("code = ?", code).
		Where("status = ?", enum.CommonYes).
		Where("is_del = ?", enum.CommonNo).
		First(&platform).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &platform, nil
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Platform, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).Model(&Platform{}).Where("is_del = ?", enum.CommonNo)
	name := strings.TrimSpace(query.Name)
	if name != "" {
		db = db.Where("name LIKE ?", "%"+name+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Platform
	err := db.Order("id asc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Platform, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}

	var row Platform
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

func (r *GormRepository) PlatformsByIDs(ctx context.Context, ids []int64) (map[int64]Platform, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return map[int64]Platform{}, nil
	}

	var rows []Platform
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64]Platform, len(rows))
	for _, row := range rows {
		result[row.ID] = row
	}
	return result, nil
}

func (r *GormRepository) ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Model(&Platform{}).
		Where("code = ?", code).
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

func (r *GormRepository) Create(ctx context.Context, row Platform) (int64, error) {
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
		Model(&Platform{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
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
		Model(&Platform{}).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}
