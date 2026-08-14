package systemsetting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/shared/enum"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrRepositoryNotConfigured = errors.New("system setting repository is not configured")
	ErrDuplicateKey            = errors.New("system setting key already exists")
)

const systemSettingCacheTTL = 5 * time.Minute

type settingCache interface {
	Get(context.Context, string) (string, error)
	Set(context.Context, string, string, time.Duration) error
	Delete(context.Context, string) error
}

type redisSettingCache struct {
	client *redis.Client
}

func (c *redisSettingCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *redisSettingCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *redisSettingCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

type Repository interface {
	List(ctx context.Context, request ListRequest) ([]Setting, int64, error)
	Get(ctx context.Context, id int64) (*Setting, error)
	SettingsByIDs(ctx context.Context, ids []int64) (map[int64]Setting, error)
	FindByKey(ctx context.Context, key string) (*Setting, error)
	Restore(ctx context.Context, id int64, row Setting) (bool, error)
	Create(ctx context.Context, row Setting) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	Delete(ctx context.Context, ids []int64) error
	InvalidateCache(ctx context.Context, key string)
}

type GormRepository struct {
	db    *gorm.DB
	cache settingCache
}

func NewGormRepository(client *database.Client, cache *redisclient.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	var settingCacheClient settingCache
	if cache != nil && cache.Redis != nil {
		settingCacheClient = &redisSettingCache{client: cache.Redis}
	}
	return &GormRepository{db: client.Gorm, cache: settingCacheClient}
}

func (r *GormRepository) List(ctx context.Context, request ListRequest) ([]Setting, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).Model(&Setting{}).Where("is_del = ?", enum.CommonNo)
	key := strings.TrimSpace(request.Key)
	if key != "" {
		db = db.Where("setting_key LIKE ?", key+"%")
	}
	if request.Status != nil {
		db = db.Where("status = ?", *request.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Setting
	err := db.Order("id desc").
		Limit(request.PageSize).
		Offset((request.CurrentPage - 1) * request.PageSize).
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

	if row := r.cachedSetting(ctx, key); row != nil {
		return row, nil
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
	r.cacheSetting(ctx, row)
	return &row, nil
}

func (r *GormRepository) cachedSetting(ctx context.Context, key string) *Setting {
	if r.cache == nil {
		return nil
	}
	payload, err := r.cache.Get(ctx, cacheKey(key))
	if err != nil {
		return nil
	}
	var row Setting
	if json.Unmarshal([]byte(payload), &row) != nil || row.SettingKey != key || row.IsDel != enum.CommonNo {
		return nil
	}
	return &row
}

func (r *GormRepository) cacheSetting(ctx context.Context, row Setting) {
	if r.cache == nil {
		return
	}
	payload, err := json.Marshal(row)
	if err != nil {
		return
	}
	_ = r.cache.Set(ctx, cacheKey(row.SettingKey), string(payload), systemSettingCacheTTL)
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

func (r *GormRepository) FindByKey(ctx context.Context, key string) (*Setting, error) {
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
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Restore(ctx context.Context, id int64, row Setting) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonYes).
		Updates(map[string]any{
			"setting_value": row.SettingValue,
			"value_type":    row.ValueType,
			"remark":        row.Remark,
			"status":        enum.CommonYes,
			"is_del":        enum.CommonNo,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Setting) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return 0, ErrDuplicateKey
		}
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

func (r *GormRepository) InvalidateCache(ctx context.Context, key string) {
	if r == nil || r.cache == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	_ = r.cache.Delete(ctx, cacheKey(key))
}

func cacheKey(key string) string {
	return "sys_setting_raw_" + strings.ReplaceAll(key, ".", "_")
}
