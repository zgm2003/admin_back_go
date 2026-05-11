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
	WithTx(ctx context.Context, fn func(Repository) error) error
	FindCredentialByEmail(ctx context.Context, email string) (*UserCredential, error)
	FindCredentialByPhone(ctx context.Context, phone string) (*UserCredential, error)
	FindCredentialByID(ctx context.Context, id int64) (*UserCredential, error)
	FindDefaultRole(ctx context.Context) (*DefaultRole, error)
	CreateUser(ctx context.Context, input CreateUserInput) (int64, error)
	CreateProfile(ctx context.Context, input CreateProfileInput) error
	UpdatePassword(ctx context.Context, userID int64, passwordHash string) error
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

func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx})
	})
}

func (r *GormRepository) FindCredentialByEmail(ctx context.Context, email string) (*UserCredential, error) {
	return r.findCredential(ctx, "email", email)
}

func (r *GormRepository) FindCredentialByPhone(ctx context.Context, phone string) (*UserCredential, error) {
	return r.findCredential(ctx, "phone", phone)
}

func (r *GormRepository) FindCredentialByID(ctx context.Context, id int64) (*UserCredential, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}

	var user UserCredential
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
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

func (r *GormRepository) FindDefaultRole(ctx context.Context) (*DefaultRole, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var row DefaultRole
	err := r.db.WithContext(ctx).
		Where("is_default = ?", commonYes).
		Where("is_del = ?", commonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) CreateUser(ctx context.Context, input CreateUserInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	now := time.Now()
	row := userCreateRow{
		RoleID:    input.RoleID,
		Username:  input.Username,
		Email:     input.Email,
		Phone:     input.Phone,
		Status:    commonYes,
		IsDel:     commonNo,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) CreateProfile(ctx context.Context, input CreateProfileInput) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	now := time.Now()
	return r.db.WithContext(ctx).Table("user_profiles").Create(map[string]any{
		"user_id":    input.UserID,
		"sex":        input.Sex,
		"is_del":     commonNo,
		"created_at": now,
		"updated_at": now,
	}).Error
}

func (r *GormRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if userID <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&UserCredential{}).
		Where("id = ?", userID).
		Where("is_del = ?", commonNo).
		Updates(map[string]any{
			"password":   passwordHash,
			"updated_at": time.Now(),
		}).Error
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
