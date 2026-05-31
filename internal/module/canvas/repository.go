package canvas

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("canvas repository not configured")

type Repository interface {
	ListPrompts(ctx context.Context, query PromptListQuery) ([]Prompt, int64, error)
	CreatePrompt(ctx context.Context, row Prompt) (int64, error)
	SoftDeletePrompt(ctx context.Context, id int64) error
	ListAssets(ctx context.Context, query AssetListQuery) ([]Asset, int64, error)
	CreateAsset(ctx context.Context, row Asset) (int64, error)
	SoftDeleteAsset(ctx context.Context, id int64) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ListPrompts(ctx context.Context, query PromptListQuery) ([]Prompt, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	query = normalizePromptListQuery(query)
	db := r.db.WithContext(ctx).Model(&Prompt{}).Where("is_del = ?", query.IsDel)
	if query.Status > 0 {
		db = db.Where("status = ?", query.Status)
	}
	if query.Category != "" {
		db = db.Where("category = ?", query.Category)
	}
	if query.Keyword != "" {
		like := "%" + query.Keyword + "%"
		db = db.Where("(title LIKE ? OR slug LIKE ? OR prompt LIKE ?)", like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Prompt
	err := db.Order("updated_at DESC, id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) CreatePrompt(ctx context.Context, row Prompt) (int64, error) {
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

func (r *GormRepository) SoftDeletePrompt(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Prompt{}).Where("id = ? AND is_del = ?", id, IsDelActive).Updates(map[string]any{"is_del": IsDelDeleted, "updated_at": time.Now()}).Error
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

func normalizePromptListQuery(query PromptListQuery) PromptListQuery {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Category = strings.TrimSpace(query.Category)
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
