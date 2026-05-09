package aiapp

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiapp repository not configured")

type AppWithEngine struct {
	App
	ProviderName    string `gorm:"column:provider_name"`
	EngineType      string `gorm:"column:engine_type"`
	EngineBaseURL   string `gorm:"column:engine_base_url"`
	EngineAPIKeyEnc string `gorm:"column:engine_api_key_enc"`
	EngineStatus    int    `gorm:"column:engine_status"`
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]AppWithEngine, int64, error)
	Get(ctx context.Context, id uint64) (*AppWithEngine, error)
	GetRaw(ctx context.Context, id uint64) (*App, error)
	ListActiveProviders(ctx context.Context) ([]Provider, error)
	GetActiveProvider(ctx context.Context, id uint64) (*Provider, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error)
	Create(ctx context.Context, row App) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	Delete(ctx context.Context, id uint64) error
	ListBindings(ctx context.Context, appID uint64) ([]Binding, error)
	ExistsBinding(ctx context.Context, appID uint64, bindType string, bindKey string, excludeID uint64) (bool, error)
	CreateBinding(ctx context.Context, row Binding) (uint64, error)
	DeleteBinding(ctx context.Context, id uint64) error
	ListVisibleApps(ctx context.Context, query OptionQuery) ([]App, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]AppWithEngine, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.appSelectDB(ctx)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("a.name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.Code) != "" {
		db = db.Where("a.code = ?", strings.TrimSpace(query.Code))
	}
	if strings.TrimSpace(query.AppType) != "" {
		db = db.Where("a.app_type = ?", strings.TrimSpace(query.AppType))
	}
	if query.ProviderID > 0 {
		db = db.Where("a.provider_id = ?", query.ProviderID)
	}
	if query.Status != nil {
		db = db.Where("a.status = ?", *query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AppWithEngine
	if err := db.Order("a.id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id uint64) (*AppWithEngine, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row AppWithEngine
	err := r.appSelectDB(ctx).Where("a.id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) GetRaw(ctx context.Context, id uint64) (*App, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row App
	err := r.activeApps(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ListActiveProviders(ctx context.Context) ([]Provider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Provider
	err := r.db.WithContext(ctx).Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetActiveProvider(ctx context.Context, id uint64) (*Provider, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Provider
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ? AND status = ?", id, enum.CommonNo, enum.CommonYes).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeApps(ctx).Model(&App{}).Where("code = ?", strings.TrimSpace(code))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row App) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeApps(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeApps(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeApps(ctx).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) ListBindings(ctx context.Context, appID uint64) ([]Binding, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Binding
	err := r.db.WithContext(ctx).Where("is_del = ? AND app_id = ?", enum.CommonNo, appID).Order("sort ASC, id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) ExistsBinding(ctx context.Context, appID uint64, bindType string, bindKey string, excludeID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Binding{}).Where("is_del = ? AND app_id = ? AND bind_type = ? AND bind_key = ?", enum.CommonNo, appID, strings.TrimSpace(bindType), strings.TrimSpace(bindKey))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CreateBinding(ctx context.Context, row Binding) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) DeleteBinding(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Binding{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) ListVisibleApps(ctx context.Context, query OptionQuery) ([]App, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []App
	db := r.activeApps(ctx).Where("status = ?", enum.CommonYes)
	if query.UserID > 0 || query.RoleID > 0 || strings.TrimSpace(query.Platform) != "" {
		db = db.Where(`EXISTS (
			SELECT 1 FROM ai_app_bindings b
			WHERE b.app_id = ai_apps.id AND b.is_del = ? AND b.status = ? AND (
				(b.bind_type = 'user' AND b.bind_key = ?) OR
				(b.bind_type = 'role' AND b.bind_key = ?) OR
				(b.bind_type = 'menu') OR
				(b.bind_type = 'scene') OR
				(b.bind_type = 'permission')
			)
		) OR NOT EXISTS (SELECT 1 FROM ai_app_bindings b2 WHERE b2.app_id = ai_apps.id AND b2.is_del = ? AND b2.status = ?)`, enum.CommonNo, enum.CommonYes, uintKey(query.UserID), uintKey(query.RoleID), enum.CommonNo, enum.CommonYes)
	}
	if err := db.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) appSelectDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_apps AS a").Select(`a.*, e.name AS provider_name, e.engine_type AS engine_type, e.base_url AS engine_base_url, e.api_key_enc AS engine_api_key_enc, e.status AS engine_status`).Joins("LEFT JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ?", enum.CommonNo).Where("a.is_del = ?", enum.CommonNo)
}

func (r *GormRepository) activeApps(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}

func uintKey(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
