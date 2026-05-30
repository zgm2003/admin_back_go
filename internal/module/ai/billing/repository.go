package aibilling

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aibilling repository not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Rule, int64, error)
	Get(ctx context.Context, id uint64) (*Rule, error)
	GetByScene(ctx context.Context, scene string) (*Rule, error)
	EnabledByScene(ctx context.Context, scene string) (*Rule, error)
	Create(ctx context.Context, row Rule) (uint64, error)
	Update(ctx context.Context, id uint64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id uint64, status int) error
	Delete(ctx context.Context, id uint64) error
	CreateRecord(ctx context.Context, row BillingRecord) (int64, error)
	GetRecord(ctx context.Context, id int64) (*BillingRecord, error)
	UpdateRecord(ctx context.Context, id int64, fields map[string]any) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Rule, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeDB(ctx)
	if scene := strings.TrimSpace(query.Scene); scene != "" {
		db = db.Where("scene = ?", scene)
	}
	if unit := strings.TrimSpace(query.Unit); unit != "" {
		db = db.Where("unit = ?", unit)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	var total int64
	if err := db.Model(&Rule{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Rule
	if err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Get(ctx context.Context, id uint64) (*Rule, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row Rule
	err := r.activeDB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) GetByScene(ctx context.Context, scene string) (*Rule, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Rule
	err := r.activeDB(ctx).Where("scene = ?", strings.TrimSpace(scene)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) EnabledByScene(ctx context.Context, scene string) (*Rule, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Rule
	err := r.activeDB(ctx).Where("scene = ?", strings.TrimSpace(scene)).Where("status = ?", RuleStatusEnabled).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Create(ctx context.Context, row Rule) (uint64, error) {
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
	return r.activeDB(ctx).Model(&Rule{}).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Model(&Rule{}).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) Delete(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDB(ctx).Model(&Rule{}).Where("id = ?", id).Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) CreateRecord(ctx context.Context, row BillingRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) GetRecord(ctx context.Context, id int64) (*BillingRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id == 0 {
		return nil, nil
	}
	var row BillingRecord
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateRecord(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&BillingRecord{}).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) activeDB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
