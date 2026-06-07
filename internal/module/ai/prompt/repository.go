package prompt

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("ai prompt repository not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Prompt, int64, error)
	Create(ctx context.Context, row Prompt) (int64, error)
	SoftDelete(ctx context.Context, id int64) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Prompt, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	query = normalizeListQuery(query)
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
	for _, tag := range query.Tags {
		db = db.Where("JSON_CONTAINS(CAST(tags_json AS JSON), JSON_QUOTE(?))", tag)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Prompt
	err := db.Order("updated_at DESC, id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) Create(ctx context.Context, row Prompt) (int64, error) {
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

func (r *GormRepository) SoftDelete(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Prompt{}).Where("id = ? AND is_del = ?", id, IsDelActive).Updates(map[string]any{"is_del": IsDelDeleted, "updated_at": time.Now()}).Error
}

func normalizeListQuery(query ListQuery) ListQuery {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Category = strings.TrimSpace(query.Category)
	query.Tags = normalizedTags(query.Tags)
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

func normalizedTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}
