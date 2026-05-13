package session

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

const commonNo = 2

type Repository interface {
	FindValidByAccessHash(ctx context.Context, accessHash string, now time.Time) (*Session, error)
	FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error)
	FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error)
	FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error)
	Create(ctx context.Context, input SessionCreate) (int64, error)
	ListActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error)
	RevokeByUserPlatform(ctx context.Context, userID int64, platform string, revokedAt time.Time) error
	UpdateAccessToken(ctx context.Context, sessionID int64, accessHash string, expiresAt time.Time) error
	Rotate(ctx context.Context, sessionID int64, rotation Rotation) error
	Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error
}

type SessionCreate struct {
	UserID           int64
	AccessTokenHash  string
	RefreshTokenHash string
	Platform         string
	DeviceID         string
	IP               string
	UserAgent        string
	LastSeenAt       time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
}

type Rotation struct {
	AccessTokenHash  string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	LastSeenAt       time.Time
	IP               string
	UserAgent        string
}

func (r *GormRepository) Create(ctx context.Context, input SessionCreate) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}

	row := Session{
		UserID:           input.UserID,
		AccessTokenHash:  input.AccessTokenHash,
		RefreshTokenHash: input.RefreshTokenHash,
		Platform:         input.Platform,
		DeviceID:         input.DeviceID,
		IP:               input.IP,
		UserAgent:        input.UserAgent,
		LastSeenAt:       input.LastSeenAt,
		ExpiresAt:        input.ExpiresAt,
		RefreshExpiresAt: input.RefreshExpiresAt,
		IsDel:            commonNo,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) ListActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var sessions []Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		Order("id ASC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *GormRepository) RevokeByUserPlatform(ctx context.Context, userID int64, platform string, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Update("revoked_at", revokedAt).Error
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

func (r *GormRepository) FindValidByAccessHash(ctx context.Context, accessHash string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("access_token_hash = ?", accessHash).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("expires_at > ?", now).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *GormRepository) FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("id = ?", sessionID).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("expires_at > ?", now).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *GormRepository) FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("refresh_token_hash = ?", refreshHash).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *GormRepository) FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		Order("id DESC").
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *GormRepository) Rotate(ctx context.Context, sessionID int64, rotation Rotation) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"access_token_hash":  rotation.AccessTokenHash,
			"refresh_token_hash": rotation.RefreshTokenHash,
			"expires_at":         rotation.ExpiresAt,
			"refresh_expires_at": rotation.RefreshExpiresAt,
			"last_seen_at":       rotation.LastSeenAt,
			"ip":                 rotation.IP,
			"ua":                 rotation.UserAgent,
		}).Error
}

func (r *GormRepository) UpdateAccessToken(ctx context.Context, sessionID int64, accessHash string, expiresAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"access_token_hash": accessHash,
			"expires_at":        expiresAt,
		}).Error
}

func (r *GormRepository) Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Update("revoked_at", revokedAt).Error
}
