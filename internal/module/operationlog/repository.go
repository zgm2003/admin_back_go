package operationlog

import (
	"context"
	"errors"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("operation log repository not configured")

type Repository interface {
	Create(ctx context.Context, row Log) error
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

func (r *GormRepository) Create(ctx context.Context, row Log) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Create(&row).Error
}
