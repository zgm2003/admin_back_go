package canvas

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("canvas repository not configured")

type Repository interface {
	ListAssets(ctx context.Context, query AssetListQuery) ([]Asset, int64, error)
	CreateAsset(ctx context.Context, row Asset) (int64, error)
	SoftDeleteAsset(ctx context.Context, id int64) error
	ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ListAssets(ctx context.Context, query AssetListQuery) ([]Asset, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	query = normalizeAssetListQuery(query)
	db := r.db.WithContext(ctx).Model(&Asset{}).Where("is_del = ?", query.IsDel)
	if query.Status > 0 {
		db = db.Where("status = ?", query.Status)
	}
	if query.Type != "" {
		db = db.Where("type = ?", query.Type)
	}
	if query.Keyword != "" {
		like := "%" + query.Keyword + "%"
		db = db.Where("(title LIKE ? OR slug LIKE ? OR description LIKE ?)", like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Asset
	err := db.Order("updated_at DESC, id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) CreateAsset(ctx context.Context, row Asset) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if row.Status == 0 {
		row.Status = StatusEnabled
	}
	if row.IsDel == 0 {
		row.IsDel = IsDelActive
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) SoftDeleteAsset(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Asset{}).Where("id = ? AND is_del = ?", id, IsDelActive).Updates(map[string]any{"is_del": IsDelDeleted, "updated_at": time.Now()}).Error
}

func (r *GormRepository) ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return []CanvasAgentOption{}, nil
	}
	var rows []CanvasAgentOption
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS id,
			a.name AS name,
			a.avatar AS avatar,
			a.model_id AS model_id,
			COALESCE(NULLIF(a.model_display_name, ''), NULLIF(m.display_name, ''), a.model_id) AS model_display_name,
			? AS scene`, scene).
		Joins("JOIN ai_providers AS p ON p.id = a.provider_id AND p.is_del = ? AND p.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models AS m ON m.provider_id = a.provider_id AND m.model_id = a.model_id AND m.status = ?", enum.CommonYes).
		Where("a.is_del = ? AND a.status = ?", enum.CommonNo, enum.CommonYes).
		Where("JSON_CONTAINS(a.scenes_json, JSON_QUOTE(?))", scene).
		Order("a.id DESC").
		Scan(&rows).Error
	if rows == nil {
		rows = []CanvasAgentOption{}
	}
	return rows, err
}

func normalizeAssetListQuery(query AssetListQuery) AssetListQuery {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Type = strings.TrimSpace(query.Type)
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	if query.IsDel == 0 {
		query.IsDel = IsDelActive
	}
	return query
}
