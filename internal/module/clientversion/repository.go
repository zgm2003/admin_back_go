package clientversion

import (
	"context"
	"errors"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Version, int64, error)
	Get(ctx context.Context, id int64) (*Version, error)
	Latest(ctx context.Context, platform string) (*Version, error)
	FindByVersionPlatform(ctx context.Context, version string, platform string) (*Version, error)
	ExistsByVersionPlatform(ctx context.Context, version string, platform string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Version) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	SoftDelete(ctx context.Context, id int64) error
	ClearLatestByPlatform(ctx context.Context, platform string) error
	SetLatest(ctx context.Context, id int64) error
	WithTransaction(ctx context.Context, fn func(ctx context.Context, repo Repository) error) error
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Version, int64, error) {
	db := r.activeDB(ctx)
	if query.Platform != "" {
		db = db.Where("platform = ?", query.Platform)
	}
	var total int64
	if err := db.Model(&Version{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Version
	err := db.Order("id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Version, error) {
	var row Version
	err := r.activeDB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Latest(ctx context.Context, platform string) (*Version, error) {
	var row Version
	err := r.activeDB(ctx).Where("platform = ? AND is_latest = ?", platform, enum.CommonYes).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) FindByVersionPlatform(ctx context.Context, version string, platform string) (*Version, error) {
	var row Version
	err := r.activeDB(ctx).Where("version = ? AND platform = ?", version, platform).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ExistsByVersionPlatform(ctx context.Context, version string, platform string, excludeID int64) (bool, error) {
	db := r.activeDB(ctx).Model(&Version{}).Where("version = ? AND platform = ?", version, platform)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Version) (int64, error) {
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	return r.activeDB(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) SoftDelete(ctx context.Context, id int64) error {
	return r.activeDB(ctx).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) ClearLatestByPlatform(ctx context.Context, platform string) error {
	return r.activeDB(ctx).Where("platform = ?", platform).Update("is_latest", enum.CommonNo).Error
}

func (r *GormRepository) SetLatest(ctx context.Context, id int64) error {
	return r.activeDB(ctx).Where("id = ?", id).Update("is_latest", enum.CommonYes).Error
}

func (r *GormRepository) WithTransaction(ctx context.Context, fn func(ctx context.Context, repo Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &GormRepository{db: tx})
	})
}

func (r *GormRepository) activeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
