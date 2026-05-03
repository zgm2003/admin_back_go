package auth

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("auth repository not configured")

type Repository interface {
	FindCredentialByEmail(ctx context.Context, email string) (*UserCredential, error)
	FindCredentialByPhone(ctx context.Context, phone string) (*UserCredential, error)
	RecordLoginAttempt(ctx context.Context, attempt LoginAttempt) error
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

func (r *GormRepository) FindCredentialByEmail(ctx context.Context, email string) (*UserCredential, error) {
	return r.findCredential(ctx, "email", email)
}

func (r *GormRepository) FindCredentialByPhone(ctx context.Context, phone string) (*UserCredential, error) {
	return r.findCredential(ctx, "phone", phone)
}

func (r *GormRepository) findCredential(ctx context.Context, column string, value string) (*UserCredential, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var user UserCredential
	err := r.db.WithContext(ctx).
		Where(column+" = ?", value).
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

func (r *GormRepository) RecordLoginAttempt(ctx context.Context, attempt LoginAttempt) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	now := time.Now()
	row := loginAttemptRow{
		UserID:       attempt.UserID,
		LoginAccount: attempt.LoginAccount,
		LoginType:    attempt.LoginType,
		Platform:     attempt.Platform,
		IP:           attempt.IP,
		UserAgent:    attempt.UserAgent,
		IsSuccess:    attempt.IsSuccess,
		Reason:       attempt.Reason,
		IsDel:        commonNo,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}
