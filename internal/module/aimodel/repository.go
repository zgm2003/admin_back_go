package aimodel

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aimodel repository not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Model, int64, error)
	Get(ctx context.Context, id int64) (*Model, error)
	ExistsByDriverName(ctx context.Context, driver string, name string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Model) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id int64, status int) error
	Delete(ctx context.Context, id int64) error
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Model, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeDB(ctx)
	name := strings.TrimSpace(query.Name)
	if name != "" {
		db = db.Where("name LIKE ?", name+"%")
	}
	if strings.TrimSpace(query.Driver) != "" {
		db = db.Where("driver = ?", strings.TrimSpace(query.Driver))
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Model(&Model{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Model
	if err := db.Order("id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Model, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Model
	err := r.activeDB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) ExistsByDriverName(ctx context.Context, driver string, name string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.activeDB(ctx).
		Model(&Model{}).
		Where("driver = ?", strings.TrimSpace(driver)).
		Where("name = ?", strings.TrimSpace(name))
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Model) (int64, error) {
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

func (r *GormRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) activeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
