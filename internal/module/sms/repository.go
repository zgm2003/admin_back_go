package sms

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/platform/database"
	"admin_back_go/internal/platform/redisclient"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	DefaultConfig(ctx context.Context) (*Config, error)
	SaveDefaultConfig(ctx context.Context, row Config) error
	SoftDeleteDefaultConfig(ctx context.Context) error
	UpdateConfigTestResult(ctx context.Context, at *time.Time, errorMessage string) error
	ListTemplates(ctx context.Context) ([]Template, error)
	TemplateByID(ctx context.Context, id uint64) (*Template, error)
	TemplateByScene(ctx context.Context, scene string) (*Template, error)
	SaveTemplate(ctx context.Context, row Template) (uint64, error)
	UpdateTemplate(ctx context.Context, id uint64, update TemplateUpdate) error
	SoftDeleteTemplate(ctx context.Context, id uint64) error
	CreateLog(ctx context.Context, row Log) (uint64, error)
	FinishLog(ctx context.Context, id uint64, finish LogFinish) error
	ListLogs(ctx context.Context, query LogQuery) ([]Log, int64, error)
	LogByID(ctx context.Context, id uint64) (*Log, error)
	SoftDeleteLogs(ctx context.Context, ids []uint64) error
	SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error)
	SaveSetting(ctx context.Context, row systemsetting.Setting) error
	InvalidateSettingCache(ctx context.Context, key string) error
}

type GormRepository struct {
	db    *gorm.DB
	cache *redisclient.Client
}

func NewGormRepository(client *database.Client, cache ...*redisclient.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	repo := &GormRepository{db: client.Gorm}
	if len(cache) > 0 {
		repo.cache = cache[0]
	}
	return repo
}

func (r *GormRepository) DefaultConfig(ctx context.Context) (*Config, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Config
	err := r.db.WithContext(ctx).Where("config_key = ?", defaultConfigKey).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) SaveDefaultConfig(ctx context.Context, row Config) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	row.ConfigKey = defaultConfigKey
	row.IsDel = enum.CommonNo
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Config
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("config_key = ?", defaultConfigKey).First(&existing).Error
		fields := map[string]any{
			"secret_id_enc":   row.SecretIDEnc,
			"secret_id_hint":  row.SecretIDHint,
			"secret_key_enc":  row.SecretKeyEnc,
			"secret_key_hint": row.SecretKeyHint,
			"sms_sdk_app_id":  row.SmsSdkAppID,
			"sign_name":       row.SignName,
			"region":          row.Region,
			"endpoint":        row.Endpoint,
			"status":          row.Status,
			"is_del":          enum.CommonNo,
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&Config{}).Where("id = ?", existing.ID).Updates(fields).Error
	})
}

func (r *GormRepository) SoftDeleteDefaultConfig(ctx context.Context) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Config{}).Where("config_key = ?", defaultConfigKey).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) UpdateConfigTestResult(ctx context.Context, at *time.Time, errorMessage string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Config{}).Where("config_key = ?", defaultConfigKey).Where("is_del = ?", enum.CommonNo).Updates(map[string]any{"last_test_at": at, "last_test_error": truncate(errorMessage, 500)}).Error
}

func (r *GormRepository) ListTemplates(ctx context.Context) ([]Template, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Template
	err := r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) TemplateByID(ctx context.Context, id uint64) (*Template, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Template
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) TemplateByScene(ctx context.Context, scene string) (*Template, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return nil, nil
	}
	var row Template
	err := r.db.WithContext(ctx).Where("scene = ?", scene).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) SaveTemplate(ctx context.Context, row Template) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	row.IsDel = enum.CommonNo
	var savedID uint64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Template
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("scene = ?", row.Scene).First(&existing).Error
		fields := map[string]any{
			"scene":                 row.Scene,
			"name":                  row.Name,
			"tencent_template_id":   row.TencentTemplateID,
			"variables_json":        row.VariablesJSON,
			"sample_variables_json": row.SampleVariablesJSON,
			"status":                row.Status,
			"is_del":                enum.CommonNo,
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			savedID = row.ID
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&Template{}).Where("id = ?", existing.ID).Updates(fields).Error; err != nil {
			return err
		}
		savedID = existing.ID
		return nil
	})
	return savedID, err
}

func (r *GormRepository) UpdateTemplate(ctx context.Context, id uint64, update TemplateUpdate) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).Model(&Template{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Updates(map[string]any{
		"scene":                 update.Scene,
		"name":                  update.Name,
		"tencent_template_id":   update.TencentTemplateID,
		"variables_json":        update.VariablesJSON,
		"sample_variables_json": update.SampleVariablesJSON,
		"status":                update.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GormRepository) SoftDeleteTemplate(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Template{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) CreateLog(ctx context.Context, row Log) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	row.IsDel = enum.CommonNo
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) FinishLog(ctx context.Context, id uint64, finish LogFinish) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	fields := map[string]any{
		"status":             finish.Status,
		"tencent_request_id": truncate(finish.RequestID, 128),
		"tencent_serial_no":  truncate(finish.SerialNo, 128),
		"tencent_fee":        finish.Fee,
		"error_code":         truncate(finish.ErrorCode, 128),
		"error_message":      truncate(finish.ErrorMessage, 500),
		"duration_ms":        finish.DurationMS,
		"sent_at":            finish.SentAt,
	}
	return r.db.WithContext(ctx).Model(&Log{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Updates(fields).Error
}

func (r *GormRepository) ListLogs(ctx context.Context, query LogQuery) ([]Log, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Log{}).Where("is_del = ?", enum.CommonNo)
	if query.Scene != "" {
		db = db.Where("scene = ?", query.Scene)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.ToPhone != "" {
		db = db.Where("to_phone LIKE ?", strings.TrimSpace(query.ToPhone)+"%")
	}
	if query.CreatedAtStart != nil {
		db = db.Where("created_at >= ?", *query.CreatedAtStart)
	}
	if query.CreatedAtEnd != nil {
		db = db.Where("created_at <= ?", *query.CreatedAtEnd)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Log
	err := db.Order("created_at DESC, id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) LogByID(ctx context.Context, id uint64) (*Log, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Log
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) SoftDeleteLogs(ctx context.Context, ids []uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeUint64IDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Log{}).Where("id IN ?", ids).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	var row systemsetting.Setting
	err := r.db.WithContext(ctx).Where("setting_key = ?", key).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) SaveSetting(ctx context.Context, row systemsetting.Setting) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	row.SettingKey = strings.TrimSpace(row.SettingKey)
	row.IsDel = enum.CommonNo
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing systemsetting.Setting
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("setting_key = ?", row.SettingKey).First(&existing).Error
		fields := map[string]any{
			"setting_key":   row.SettingKey,
			"setting_value": row.SettingValue,
			"value_type":    row.ValueType,
			"remark":        row.Remark,
			"status":        row.Status,
			"is_del":        enum.CommonNo,
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&systemsetting.Setting{}).Where("id = ?", existing.ID).Updates(fields).Error
	})
}

func (r *GormRepository) InvalidateSettingCache(ctx context.Context, key string) error {
	if r == nil || r.cache == nil || r.cache.Redis == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	return r.cache.Redis.Del(ctx, "system_setting:"+key).Err()
}

func normalizeUint64IDs(ids []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(ids))
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
