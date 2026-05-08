package aitoolmap

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aitoolmap repository not configured")

type ToolMapWithEngine struct {
	ToolMap
	EngineConnectionName string `gorm:"column:engine_connection_name"`
	EngineType           string `gorm:"column:engine_type"`
	EngineAPIKeyEnc      string `gorm:"column:engine_api_key_enc"`
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ToolMapWithEngine, int64, error)
	GetRaw(ctx context.Context, id uint64) (*ToolMap, error)
	ListActiveConnections(ctx context.Context) ([]EngineConnection, error)
	GetActiveConnection(ctx context.Context, id uint64) (*EngineConnection, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error)
	ExistsPermissionCode(ctx context.Context, code string) (bool, error)
	Create(ctx context.Context, row ToolMap) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	Delete(ctx context.Context, id uint64) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ToolMapWithEngine, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.selectDB(ctx)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("t.name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.Code) != "" {
		db = db.Where("t.code = ?", strings.TrimSpace(query.Code))
	}
	if strings.TrimSpace(query.ToolType) != "" {
		db = db.Where("t.tool_type = ?", strings.TrimSpace(query.ToolType))
	}
	if strings.TrimSpace(query.RiskLevel) != "" {
		db = db.Where("t.risk_level = ?", strings.TrimSpace(query.RiskLevel))
	}
	if query.EngineConnectionID > 0 {
		db = db.Where("t.engine_connection_id = ?", query.EngineConnectionID)
	}
	if query.AppID != nil {
		db = db.Where("t.app_id = ?", *query.AppID)
	}
	if query.Status != nil {
		db = db.Where("t.status = ?", *query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ToolMapWithEngine
	if err := db.Order("t.id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) GetRaw(ctx context.Context, id uint64) (*ToolMap, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row ToolMap
	err := r.activeMaps(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ListActiveConnections(ctx context.Context) ([]EngineConnection, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []EngineConnection
	err := r.db.WithContext(ctx).Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetActiveConnection(ctx context.Context, id uint64) (*EngineConnection, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row EngineConnection
	err := r.db.WithContext(ctx).Where("id = ? AND is_del = ? AND status = ?", id, enum.CommonNo, enum.CommonYes).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeMaps(ctx).Model(&ToolMap{}).Where("code = ?", strings.TrimSpace(code))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) ExistsPermissionCode(ctx context.Context, code string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.db.WithContext(ctx).Table("permissions").Where("is_del = ? AND status = ? AND code = ?", enum.CommonNo, enum.CommonYes, strings.TrimSpace(code)).Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) Create(ctx context.Context, row ToolMap) (uint64, error) {
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
	return r.activeMaps(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeMaps(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeMaps(ctx).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) selectDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("ai_tool_maps AS t").
		Select(`t.*, e.name AS engine_connection_name, e.engine_type AS engine_type, e.api_key_enc AS engine_api_key_enc`).
		Joins("LEFT JOIN ai_engine_connections e ON e.id = t.engine_connection_id AND e.is_del = ?", enum.CommonNo).
		Where("t.is_del = ?", enum.CommonNo)
}

func (r *GormRepository) activeMaps(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
