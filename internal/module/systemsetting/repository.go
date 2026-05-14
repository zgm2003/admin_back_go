package systemsetting

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"
	"admin_back_go/internal/platform/redisclient"

	"gorm.io/gorm"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Setting, int64, error)
	Get(ctx context.Context, id int64) (*Setting, error)
	SettingsByIDs(ctx context.Context, ids []int64) (map[int64]Setting, error)
	ExistsByKey(ctx context.Context, key string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Setting) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	Delete(ctx context.Context, ids []int64) error
	InvalidateCache(ctx context.Context, key string) error
}

type GormRepository struct {
	db    *gorm.DB
	cache *redisclient.Client
}

func NewGormRepository(client *database.Client, cache *redisclient.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm, cache: cache}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Setting, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).Model(&Setting{}).Where("is_del = ?", enum.CommonNo)
	key := strings.TrimSpace(query.Key)
	if key != "" {
		db = db.Where("setting_key LIKE ?", key+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Setting
	err := db.Order("id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Setting, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}

	var row Setting
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

func (r *GormRepository) SettingByKey(ctx context.Context, key string) (*Setting, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	var row Setting
	err := r.db.WithContext(ctx).
		Where("setting_key = ?", key).
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

func (r *GormRepository) SettingsByIDs(ctx context.Context, ids []int64) (map[int64]Setting, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return map[int64]Setting{}, nil
	}

	var rows []Setting
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64]Setting, len(rows))
	for _, row := range rows {
		result[row.ID] = row
	}
	return result, nil
}

func (r *GormRepository) ExistsByKey(ctx context.Context, key string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("setting_key = ?", key).
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

func (r *GormRepository) Create(ctx context.Context, row Setting) (int64, error) {
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
		Model(&Setting{}).
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
		Model(&Setting{}).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) InvalidateCache(ctx context.Context, key string) error {
	if r == nil || r.cache == nil || r.cache.Redis == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	return r.cache.Redis.Del(ctx, cacheKey(key)).Err()
}

func cacheKey(key string) string {
	return "sys_setting_raw_" + strings.ReplaceAll(key, ".", "_")
}
