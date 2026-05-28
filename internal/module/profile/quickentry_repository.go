package profile

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrQuickEntryRepositoryNotConfigured = errors.New("user quick entry repository is not configured")

type QuickEntryRepository interface {
	ActiveAdminPagePermissionIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error)
	ReplaceForUser(ctx context.Context, userID int64, permissionIDs []int64) ([]QuickEntry, error)
}

type GormQuickEntryRepository struct {
	db *gorm.DB
}

func NewGormQuickEntryRepository(client *database.Client) QuickEntryRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormQuickEntryRepository{db: client.Gorm}
}

func (r *GormQuickEntryRepository) ActiveAdminPagePermissionIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	if r == nil || r.db == nil {
		return nil, ErrQuickEntryRepositoryNotConfigured
	}
	result := make(map[int64]struct{}, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var rows []struct {
		ID int64 `gorm:"column:id"`
	}
	err := r.db.WithContext(ctx).
		Table("permissions").
		Select("id").
		Where("id IN ?", ids).
		Where("platform = ?", enum.PlatformAdmin).
		Where("type = ?", enum.PermissionTypePage).
		Where("status = ?", enum.CommonYes).
		Where("is_del = ?", enum.CommonNo).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ID] = struct{}{}
	}
	return result, nil
}

func (r *GormQuickEntryRepository) ReplaceForUser(ctx context.Context, userID int64, permissionIDs []int64) ([]QuickEntry, error) {
	if r == nil || r.db == nil {
		return nil, ErrQuickEntryRepositoryNotConfigured
	}
	var entries []QuickEntry
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&QuickEntryModel{}).
			Where("user_id = ?", userID).
			Where("is_del = ?", enum.CommonNo).
			Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now}).Error; err != nil {
			return err
		}

		if len(permissionIDs) > 0 {
			rows := make([]QuickEntryModel, 0, len(permissionIDs))
			for index, permissionID := range permissionIDs {
				rows = append(rows, QuickEntryModel{
					UserID:       userID,
					PermissionID: permissionID,
					Sort:         index + 1,
					IsDel:        enum.CommonNo,
				})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}

		return tx.Model(&QuickEntryModel{}).
			Select("id", "permission_id", "sort").
			Where("user_id = ?", userID).
			Where("permission_id > ?", 0).
			Where("is_del = ?", enum.CommonNo).
			Order("sort asc").
			Scan(&entries).Error
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
