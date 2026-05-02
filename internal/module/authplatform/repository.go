package authplatform

import (
	"context"
	"errors"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

const commonNo = 2

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) Repository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) FindActiveByCode(ctx context.Context, code string) (*Platform, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var platform Platform
	err := r.db.WithContext(ctx).
		Where("code = ?", code).
		Where("status = ?", commonYes).
		Where("is_del = ?", commonNo).
		First(&platform).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &platform, nil
}
