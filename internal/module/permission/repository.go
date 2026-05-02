package permission

import (
	"context"
	"errors"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("permission repository not configured")

type Repository interface {
	FindRole(ctx context.Context, roleID int64) (*Role, error)
	PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
	AllActivePermissions(ctx context.Context) ([]Permission, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) Repository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) FindRole(ctx context.Context, roleID int64) (*Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var role Role
	err := r.db.WithContext(ctx).
		Where("id = ?", roleID).
		Where("is_del = ?", CommonNo).
		First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *GormRepository) PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&RolePermission{}).
		Where("role_id = ?", roleID).
		Where("is_del = ?", CommonNo).
		Order("id asc").
		Pluck("permission_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *GormRepository) AllActivePermissions(ctx context.Context) ([]Permission, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var permissions []Permission
	err := r.db.WithContext(ctx).
		Where("is_del = ?", CommonNo).
		Where("status = ?", StatusActive).
		Order("parent_id asc").
		Order("sort asc").
		Order("id asc").
		Find(&permissions).Error
	if err != nil {
		return nil, err
	}
	return permissions, nil
}
