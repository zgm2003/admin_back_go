package user

import (
	"context"
	"errors"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

const (
	commonNo = 2
)

var ErrRepositoryNotConfigured = errors.New("user repository not configured")

type Repository interface {
	FindUser(ctx context.Context, userID int64) (*User, error)
	FindProfile(ctx context.Context, userID int64) (*Profile, error)
	FindRole(ctx context.Context, roleID int64) (*Role, error)
	QuickEntries(ctx context.Context, userID int64) ([]QuickEntry, error)
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

func (r *GormRepository) FindUser(ctx context.Context, userID int64) (*User, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var user User
	err := r.db.WithContext(ctx).
		Where("id = ?", userID).
		Where("is_del = ?", commonNo).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormRepository) FindProfile(ctx context.Context, userID int64) (*Profile, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var profile Profile
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("is_del = ?", commonNo).
		First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *GormRepository) FindRole(ctx context.Context, roleID int64) (*Role, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if roleID <= 0 {
		return nil, nil
	}

	var role Role
	err := r.db.WithContext(ctx).
		Where("id = ?", roleID).
		Where("is_del = ?", commonNo).
		First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *GormRepository) QuickEntries(ctx context.Context, userID int64) ([]QuickEntry, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var entries []QuickEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("permission_id > ?", 0).
		Where("is_del = ?", commonNo).
		Order("sort asc").
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}
