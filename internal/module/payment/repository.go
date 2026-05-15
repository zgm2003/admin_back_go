package payment

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Repository interface {
	ListConfigs(ctx context.Context, query ConfigListQuery) ([]Config, int64, error)
	GetConfig(ctx context.Context, id int64) (*Config, error)
	GetConfigByCode(ctx context.Context, code string) (*Config, error)
	CreateConfig(ctx context.Context, cfg Config) (int64, error)
	UpdateConfig(ctx context.Context, cfg Config, keepPrivateKey bool) error
	ChangeConfigStatus(ctx context.Context, id int64, status int) error
	DeleteConfig(ctx context.Context, id int64) error
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

func (r *GormRepository) ListConfigs(ctx context.Context, query ConfigListQuery) ([]Config, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Model(&Config{}).Where("is_del = ?", enum.CommonNo)
	if name := strings.TrimSpace(query.Name); name != "" {
		db = db.Where("name LIKE ? OR code LIKE ? OR app_id LIKE ?", name+"%", name+"%", name+"%")
	}
	if provider := strings.TrimSpace(query.Provider); provider != "" {
		db = db.Where("provider = ?", provider)
	}
	if environment := strings.TrimSpace(query.Environment); environment != "" {
		db = db.Where("environment = ?", environment)
	}
	if query.Status > 0 {
		db = db.Where("status = ?", query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Config
	err := db.Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetConfig(ctx context.Context, id int64) (*Config, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Config
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ?", id, enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetConfigByCode(ctx context.Context, code string) (*Config, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Config
	err := r.db.WithContext(ctx).Where("code = ? AND is_del = ?", strings.TrimSpace(code), enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateConfig(ctx context.Context, cfg Config) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	cfg.IsDel = enum.CommonNo
	if err := r.db.WithContext(ctx).Create(&cfg).Error; err != nil {
		return 0, err
	}
	return cfg.ID, nil
}

func (r *GormRepository) UpdateConfig(ctx context.Context, cfg Config, keepPrivateKey bool) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	fields := map[string]any{
		"provider":             cfg.Provider,
		"name":                 cfg.Name,
		"app_id":               cfg.AppID,
		"app_cert_path":        cfg.AppCertPath,
		"platform_cert_path":   cfg.PlatformCertPath,
		"root_cert_path":       cfg.RootCertPath,
		"notify_url":           cfg.NotifyURL,
		"environment":          cfg.Environment,
		"enabled_methods_json": cfg.EnabledMethodsJSON,
		"status":               cfg.Status,
		"remark":               cfg.Remark,
	}
	if !keepPrivateKey {
		fields["private_key_enc"] = cfg.PrivateKeyEnc
		fields["private_key_hint"] = cfg.PrivateKeyHint
	}
	return r.db.WithContext(ctx).Model(&Config{}).Where("id = ? AND is_del = ?", cfg.ID, enum.CommonNo).Updates(fields).Error
}

func (r *GormRepository) ChangeConfigStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Config{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("status", status).Error
}

func (r *GormRepository) DeleteConfig(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Config{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{
		"is_del": enum.CommonYes,
		"status": enum.CommonNo,
	}).Error
}

func normalizePage(page int, size int) (int, int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return page, size, (page - 1) * size
}
