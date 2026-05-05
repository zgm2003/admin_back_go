package notificationtask

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

var ErrRepositoryNotConfigured = errors.New("notification task repository is not configured")

const (
	pendingDispatchSendAtCondition = "(send_at IS NULL OR send_at <= ?)"
	pendingDispatchOrder           = "CASE WHEN send_at IS NULL THEN 0 ELSE 1 END asc, send_at asc, id asc"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Task, int64, error)
	CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error)
	Create(ctx context.Context, row Task) (int64, error)
	Get(ctx context.Context, id int64) (*Task, error)
	CancelPending(ctx context.Context, id int64) (int64, error)
	Delete(ctx context.Context, id int64) (int64, error)
	CountTargetUsers(ctx context.Context, targetType int, targetIDs []int64) (int, error)
	ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]int64, error)
	ClaimSendTask(ctx context.Context, id int64) (*Task, bool, error)
	TargetUserIDs(ctx context.Context, task Task) ([]int64, error)
	InsertNotifications(ctx context.Context, rows []Notification) error
	UpdateProgress(ctx context.Context, id int64, sentCount int, totalCount int) error
	MarkSuccess(ctx context.Context, id int64, sentCount int, totalCount int) error
	MarkFailed(ctx context.Context, id int64, errMsg string) error
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

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Task, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Task{}).Where("is_del = ?", enum.CommonNo)
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	title := strings.TrimSpace(query.Title)
	if title != "" {
		db = db.Where("title LIKE ?", title+"%")
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Task
	err := db.Order("id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Task{}).Where("is_del = ?", enum.CommonNo)
	title := strings.TrimSpace(query.Title)
	if title != "" {
		db = db.Where("title LIKE ?", title+"%")
	}

	var rows []struct {
		Status int
		Num    int64
	}
	if err := db.Select("status, COUNT(*) AS num").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[int]int64, len(rows))
	for _, row := range rows {
		result[row.Status] = row.Num
	}
	return result, nil
}

func (r *GormRepository) Create(ctx context.Context, row Task) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Task, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Task
	err := r.db.WithContext(ctx).
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

func (r *GormRepository) CancelPending(ctx context.Context, id int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Where("status = ?", enum.NotificationTaskStatusPending).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes)
	return result.RowsAffected, result.Error
}

func (r *GormRepository) Delete(ctx context.Context, id int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes)
	return result.RowsAffected, result.Error
}

func (r *GormRepository) CountTargetUsers(ctx context.Context, targetType int, targetIDs []int64) (int, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Table("users").Where("is_del = ?", enum.CommonNo)
	switch targetType {
	case enum.NotificationTargetAll:
		// no extra filter
	case enum.NotificationTargetUsers:
		if len(targetIDs) == 0 {
			return 0, nil
		}
		db = db.Where("id IN ?", targetIDs)
	case enum.NotificationTargetRoles:
		if len(targetIDs) == 0 {
			return 0, nil
		}
		db = db.Where("role_id IN ?", targetIDs)
	default:
		return 0, nil
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *GormRepository) ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 100
	}

	var ids []int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Task
		if err := pendingDispatchQuery(tx, now, limit).Find(&rows).Error; err != nil {
			return err
		}
		ids = make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if len(ids) == 0 {
			return nil
		}
		return tx.Model(&Task{}).
			Where("id IN ?", ids).
			Where("status = ?", enum.NotificationTaskStatusPending).
			Where("is_del = ?", enum.CommonNo).
			Update("status", enum.NotificationTaskStatusSending).Error
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func pendingDispatchQuery(db *gorm.DB, now time.Time, limit int) *gorm.DB {
	return db.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Select("id").
		Where("status = ?", enum.NotificationTaskStatusPending).
		Where("is_del = ?", enum.CommonNo).
		Where(pendingDispatchSendAtCondition, now).
		Order(gorm.Expr(pendingDispatchOrder)).
		Limit(limit)
}

func (r *GormRepository) ClaimSendTask(ctx context.Context, id int64) (*Task, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, false, nil
	}
	var task Task
	claimed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Where("is_del = ?", enum.CommonNo).
			First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		switch task.Status {
		case enum.NotificationTaskStatusSuccess, enum.NotificationTaskStatusFailed:
			return nil
		case enum.NotificationTaskStatusPending, enum.NotificationTaskStatusSending:
			if task.Status == enum.NotificationTaskStatusPending {
				if err := tx.Model(&Task{}).
					Where("id = ?", id).
					Update("status", enum.NotificationTaskStatusSending).Error; err != nil {
					return err
				}
				task.Status = enum.NotificationTaskStatusSending
			}
			claimed = true
			return nil
		default:
			return nil
		}
	})
	if err != nil {
		return nil, false, err
	}
	if task.ID == 0 {
		return nil, false, nil
	}
	return &task, claimed, nil
}

func (r *GormRepository) TargetUserIDs(ctx context.Context, task Task) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Table("users").Where("is_del = ?", enum.CommonNo)
	switch task.TargetType {
	case enum.NotificationTargetAll:
	case enum.NotificationTargetUsers:
		ids := decodeIDs(task.TargetIDs)
		if len(ids) == 0 {
			return []int64{}, nil
		}
		db = db.Where("id IN ?", ids)
	case enum.NotificationTargetRoles:
		ids := decodeIDs(task.TargetIDs)
		if len(ids) == 0 {
			return []int64{}, nil
		}
		db = db.Where("role_id IN ?", ids)
	default:
		return []int64{}, nil
	}
	var ids []int64
	err := db.Order("id asc").Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *GormRepository) InsertNotifications(ctx context.Context, rows []Notification) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

func (r *GormRepository) UpdateProgress(ctx context.Context, id int64, sentCount int, totalCount int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Updates(map[string]any{"sent_count": sentCount, "total_count": totalCount}).Error
}

func (r *GormRepository) MarkSuccess(ctx context.Context, id int64, sentCount int, totalCount int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      enum.NotificationTaskStatusSuccess,
			"sent_count":  sentCount,
			"total_count": totalCount,
			"error_msg":   "",
		}).Error
}

func (r *GormRepository) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Update("status", enum.NotificationTaskStatusFailed).
		Update("error_msg", truncateRunes(errMsg, 500)).Error
}
