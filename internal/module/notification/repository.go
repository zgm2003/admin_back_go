package notification

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("notification repository is not configured")

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Notification, int64, error)
	UnreadCount(ctx context.Context, userID int64, platform string) (int64, error)
	MarkRead(ctx context.Context, input MarkReadInput) (int64, error)
	Delete(ctx context.Context, input DeleteInput) (int64, error)
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Notification, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.scopeUser(ctx, query.UserID, query.Platform)
	if query.Type != nil {
		db = db.Where("type = ?", *query.Type)
	}
	if query.Level != nil {
		db = db.Where("level = ?", *query.Level)
	}
	if query.IsRead != nil {
		db = db.Where("is_read = ?", *query.IsRead)
	}
	keyword := strings.TrimSpace(query.Keyword)
	if keyword != "" {
		db = db.Where("title LIKE ?", keyword+"%")
	}

	var total int64
	if err := db.Model(&Notification{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Notification
	err := db.Select("id", "user_id", "title", "content", "type", "level", "link", "platform", "is_read", "created_at").
		Order("id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) UnreadCount(ctx context.Context, userID int64, platform string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	var count int64
	err := r.scopeUser(ctx, userID, platform).
		Model(&Notification{}).
		Where("is_read = ?", enum.CommonNo).
		Count(&count).Error
	return count, err
}

func (r *GormRepository) MarkRead(ctx context.Context, input MarkReadInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	db := r.scopeUser(ctx, input.UserID, input.Platform).
		Model(&Notification{}).
		Where("is_read = ?", enum.CommonNo)
	if len(input.IDs) > 0 {
		db = db.Where("id IN ?", input.IDs)
	}
	result := db.Update("is_read", enum.CommonYes)
	return result.RowsAffected, result.Error
}

func (r *GormRepository) Delete(ctx context.Context, input DeleteInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if len(input.IDs) == 0 {
		return 0, nil
	}
	result := r.scopeUser(ctx, input.UserID, input.Platform).
		Model(&Notification{}).
		Where("id IN ?", input.IDs).
		Update("is_del", enum.CommonYes)
	return result.RowsAffected, result.Error
}

func (r *GormRepository) scopeUser(ctx context.Context, userID int64, platform string) *gorm.DB {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("platform IN ?", []string{platform, "all"}).
		Where("is_del = ?", enum.CommonNo)
}
