package usersession

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRepositoryNotConfigured = errors.New("user session repository is not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Stats(ctx context.Context, now time.Time) ([]StatsRow, error)
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	query.Now = normalizeNow(query.Now)
	db := r.baseListQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			us.id,
			us.user_id,
			COALESCE(u.username, '') AS username,
			us.platform,
			us.device_id,
			us.ip,
			COALESCE(us.ua, '') AS user_agent,
			us.last_seen_at,
			us.created_at,
			us.expires_at,
			us.refresh_expires_at,
			us.revoked_at
		`).
		Order(clause.Expr{
			SQL: `CASE
				WHEN us.revoked_at IS NULL AND us.refresh_expires_at > ? THEN 1
				WHEN us.revoked_at IS NULL AND us.refresh_expires_at <= ? THEN 2
				ELSE 3
			END ASC`,
			Vars:               []any{query.Now, query.Now},
			WithoutParentheses: true,
		}).
		Order("us.last_seen_at DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Stats(ctx context.Context, now time.Time) ([]StatsRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	now = normalizeNow(now)
	var rows []StatsRow
	err := r.db.WithContext(ctx).
		Table("user_sessions AS us").
		Where("us.is_del = ?", enum.CommonNo).
		Where("us.revoked_at IS NULL").
		Where("us.refresh_expires_at > ?", now).
		Select("us.platform, COUNT(*) AS total").
		Group("us.platform").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) baseListQuery(ctx context.Context, query ListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("user_sessions AS us").
		Joins("LEFT JOIN users AS u ON u.id = us.user_id").
		Where("us.is_del = ?", enum.CommonNo)

	if query.Username != "" {
		db = db.Where("u.username LIKE ?", strings.TrimSpace(query.Username)+"%")
	}
	if query.Platform != "" {
		db = db.Where("us.platform = ?", query.Platform)
	}
	if query.Status != "" {
		switch query.Status {
		case SessionStatusActive:
			db = db.Where("us.revoked_at IS NULL").Where("us.refresh_expires_at > ?", query.Now)
		case SessionStatusExpired:
			db = db.Where("us.revoked_at IS NULL").Where("us.refresh_expires_at <= ?", query.Now)
		case SessionStatusRevoked:
			db = db.Where("us.revoked_at IS NOT NULL")
		}
	}
	return db
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}
