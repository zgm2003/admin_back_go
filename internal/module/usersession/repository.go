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
	GetByID(ctx context.Context, id int64) (*SessionRecord, error)
	GetByIDs(ctx context.Context, ids []int64) ([]SessionRecord, error)
	MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error)
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

func (r *GormRepository) GetByID(ctx context.Context, id int64) (*SessionRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row SessionRecord
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("id, user_id, platform, access_token_hash, revoked_at").
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) GetByIDs(ctx context.Context, ids []int64) ([]SessionRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if len(ids) == 0 {
		return []SessionRecord{}, nil
	}
	var rows []SessionRecord
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("id, user_id, platform, access_token_hash, revoked_at").
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Order("id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&SessionRecordModel{}).
		Where("id IN ?", ids).
		Where("revoked_at IS NULL").
		Where("is_del = ?", enum.CommonNo).
		Update("revoked_at", revokedAt)
	return result.RowsAffected, result.Error
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

type SessionRecordModel struct {
	ID        int64      `gorm:"column:id"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	IsDel     int        `gorm:"column:is_del"`
}

func (SessionRecordModel) TableName() string {
	return "user_sessions"
}
