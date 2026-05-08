package aiprompt

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiprompt repository not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Prompt, int64, error)
	Get(ctx context.Context, id int64) (*Prompt, error)
	Create(ctx context.Context, row Prompt) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	Delete(ctx context.Context, id int64) error
	IncrementUseCount(ctx context.Context, id int64) error
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Prompt, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeDB(ctx).Where("user_id = ?", query.UserID)
	title := strings.TrimSpace(query.Title)
	if title != "" {
		db = db.Where("title LIKE ?", "%"+title+"%")
	}
	category := strings.TrimSpace(query.Category)
	if category != "" {
		db = db.Where("category = ?", category)
	}
	if query.IsFavorite != nil && *query.IsFavorite == enum.CommonYes {
		db = db.Where("is_favorite = ?", enum.CommonYes)
	}
	var total int64
	if err := db.Model(&Prompt{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Prompt
	if err := db.Order("is_favorite DESC").Order("sort DESC").Order("use_count DESC").Order("id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Prompt, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Prompt
	err := r.activeDB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Create(ctx context.Context, row Prompt) (int64, error) {
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
	return r.activeDB(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) Delete(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) IncrementUseCount(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Where("id = ?", id).UpdateColumn("use_count", gorm.Expr("use_count + 1")).Error
}

func (r *GormRepository) activeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
