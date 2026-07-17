package auth

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/infra/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSessionRepositoryNotConfigured = errors.New("session repository is not configured")

// Session persistence model and cache serialization.
type Session struct {
	ID     int64 `gorm:"column:id"`
	UserID int64 `gorm:"column:user_id"`
	// LegacyNonce satisfies the pre-P09 unique physical column without deriving
	// or retaining any value from the signed access credential.
	LegacyNonce      string     `gorm:"column:access_token_hash"`
	RefreshTokenHash string     `gorm:"column:refresh_token_hash"`
	Platform         string     `gorm:"column:platform"`
	DeviceID         string     `gorm:"column:device_id"`
	IP               string     `gorm:"column:ip"`
	UserAgent        string     `gorm:"column:ua"`
	LastSeenAt       time.Time  `gorm:"column:last_seen_at"`
	ExpiresAt        time.Time  `gorm:"column:expires_at"`
	RefreshExpiresAt time.Time  `gorm:"column:refresh_expires_at"`
	RevokedAt        *time.Time `gorm:"column:revoked_at"`
	IsDel            int        `gorm:"column:is_del"`
	UserStatus       int        `gorm:"column:user_status;->"`
	UserIsDel        int        `gorm:"column:user_is_del;->"`
}

func (Session) TableName() string {
	return "user_sessions"
}

// Session repository.
type SessionRepository interface {
	WithUserLock(ctx context.Context, userID int64, platform string, operation func(SessionRepository) error) error
	ListActiveForUpdate(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error)
	Insert(ctx context.Context, input SessionCreate) (int64, error)
	RevokeIDs(ctx context.Context, sessionIDs []int64, revokedAt time.Time) error
	FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error)
	FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error)
	FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error)
	RotateIfRefreshHash(ctx context.Context, sessionID int64, previousHash string, rotation SessionRotation) (bool, error)
	Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error
}

type SessionCreate struct {
	UserID           int64
	LegacyNonce      string
	RefreshTokenHash string
	Platform         string
	DeviceID         string
	IP               string
	UserAgent        string
	LastSeenAt       time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
}

type SessionRotation struct {
	RefreshTokenHash string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	LastSeenAt       time.Time
	IP               string
	UserAgent        string
}

func (r *SessionGormRepository) WithUserLock(ctx context.Context, userID int64, platform string, operation func(SessionRepository) error) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}
	if operation == nil {
		return errors.New("session transaction operation is not configured")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedUser struct {
			ID int64 `gorm:"column:id"`
		}
		if err := tx.WithContext(ctx).
			Table("users").
			Select("id").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND is_del = ?", userID, commonNo).
			Take(&lockedUser).Error; err != nil {
			return err
		}
		return operation(&SessionGormRepository{db: tx})
	})
}

func (r *SessionGormRepository) Insert(ctx context.Context, input SessionCreate) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrSessionRepositoryNotConfigured
	}

	row := Session{
		UserID:           input.UserID,
		LegacyNonce:      input.LegacyNonce,
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

func (r *SessionGormRepository) ListActiveForUpdate(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}
	var sessions []Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Where("refresh_expires_at > ?", now).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Order("id ASC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *SessionGormRepository) RevokeIDs(ctx context.Context, sessionIDs []int64, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}
	if len(sessionIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id IN ?", sessionIDs).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Update("revoked_at", revokedAt).Error
}

// Compatibility methods are kept off the public repository contract while
// older package-local tests and migrations transition to the transaction API.
func (r *SessionGormRepository) Create(ctx context.Context, input SessionCreate) (int64, error) {
	return r.Insert(ctx, input)
}

func (r *SessionGormRepository) ListActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
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
	return sessions, err
}

func (r *SessionGormRepository) RevokeByUserPlatform(ctx context.Context, userID int64, platform string, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("user_id = ?", userID).
		Where("platform = ?", platform).
		Where("revoked_at IS NULL").
		Where("is_del = ?", commonNo).
		Update("revoked_at", revokedAt).Error
}

type SessionGormRepository struct {
	db *gorm.DB
}

func NewSessionGormRepository(client *database.Client) SessionRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &SessionGormRepository{db: client.Gorm}
}

func (r *SessionGormRepository) FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("user_sessions.*, users.status AS user_status, users.is_del AS user_is_del").
		Joins("JOIN users ON users.id = user_sessions.user_id").
		Where("user_sessions.id = ?", sessionID).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionGormRepository) FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
	}

	var session Session
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("user_sessions.*, users.status AS user_status, users.is_del AS user_is_del").
		Joins("JOIN users ON users.id = user_sessions.user_id").
		Where("user_sessions.refresh_token_hash = ?", refreshHash).
		Where("user_sessions.revoked_at IS NULL").
		Where("user_sessions.is_del = ?", commonNo).
		Where("user_sessions.refresh_expires_at > ?", now).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionGormRepository) FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionRepositoryNotConfigured
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

func (r *SessionGormRepository) RotateIfRefreshHash(ctx context.Context, sessionID int64, previousHash string, rotation SessionRotation) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrSessionRepositoryNotConfigured
	}

	result := r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ? AND refresh_token_hash = ?", sessionID, previousHash).
		Where("revoked_at IS NULL AND is_del = ? AND refresh_expires_at > ?", commonNo, rotation.LastSeenAt).
		Updates(map[string]any{
			"refresh_token_hash": rotation.RefreshTokenHash,
			"expires_at":         rotation.ExpiresAt,
			"last_seen_at":       rotation.LastSeenAt,
			"ip":                 rotation.IP,
			"ua":                 rotation.UserAgent,
		})
	return result.RowsAffected == 1, result.Error
}

func (r *SessionGormRepository) Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrSessionRepositoryNotConfigured
	}

	return r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ?", sessionID).
		Update("revoked_at", revokedAt).Error
}
