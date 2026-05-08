package userloginlog

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("user login log repository is not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).
		Table("users_login_log AS l").
		Joins("LEFT JOIN users AS u ON u.id = l.user_id AND u.is_del = ?", enum.CommonNo).
		Where("l.is_del = ?", enum.CommonNo)

	if query.UserID > 0 {
		db = db.Where("l.user_id = ?", query.UserID)
	}
	if query.LoginAccount != "" {
		db = db.Where("l.login_account LIKE ?", strings.TrimSpace(query.LoginAccount)+"%")
	}
	if query.LoginType != "" {
		db = db.Where("l.login_type = ?", query.LoginType)
	}
	if query.IP != "" {
		db = db.Where("l.ip LIKE ?", strings.TrimSpace(query.IP)+"%")
	}
	if query.Platform != "" {
		db = db.Where("l.platform = ?", query.Platform)
	}
	if query.IsSuccess != nil {
		db = db.Where("l.is_success = ?", *query.IsSuccess)
	}
	if query.CreatedStart != "" {
		db = db.Where("l.created_at >= ?", query.CreatedStart)
	}
	if query.CreatedEnd != "" {
		db = db.Where("l.created_at <= ?", query.CreatedEnd)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			l.id,
			l.user_id,
			COALESCE(u.username, '') AS username,
			l.login_account,
			l.login_type,
			l.platform,
			l.ip,
			COALESCE(l.ua, '') AS user_agent,
			l.is_success,
			COALESCE(l.reason, '') AS reason,
			l.created_at
		`).
		Order("l.id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
