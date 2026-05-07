package exporttask

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("export task repository not configured")

type Repository interface {
	CleanExpired(ctx context.Context, now time.Time) error
	CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error)
	List(ctx context.Context, query ListQuery) ([]Task, int64, error)
	Create(ctx context.Context, row Task) (int64, error)
	MarkSuccess(ctx context.Context, id int64, result SuccessResult) error
	MarkFailed(ctx context.Context, id int64, message string) error
	DeleteByUser(ctx context.Context, userID int64, ids []int64) error
	Get(ctx context.Context, id int64) (*Task, error)
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

func (r *GormRepository) CleanExpired(ctx context.Context, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("expire_at IS NOT NULL AND expire_at < ?", now).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now}).Error
}

func (r *GormRepository) CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []struct {
		Status int   `gorm:"column:status"`
		Num    int64 `gorm:"column:num"`
	}
	db := r.scopedQuery(ctx, query.UserID, query.Title, query.FileName)
	if err := db.Select("status, COUNT(*) AS num").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[int]int64, len(rows))
	for _, row := range rows {
		result[row.Status] = row.Num
	}
	return result, nil
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Task, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.scopedQuery(ctx, query.UserID, query.Title, query.FileName)
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
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

func (r *GormRepository) Create(ctx context.Context, row Task) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) MarkSuccess(ctx context.Context, id int64, result SuccessResult) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{
			"status":     enum.ExportTaskStatusSuccess,
			"file_name":  result.FileName,
			"file_url":   result.FileURL,
			"file_size":  result.FileSize,
			"row_count":  result.RowCount,
			"error_msg":  "",
			"updated_at": now,
		}).Error
}

func (r *GormRepository) MarkFailed(ctx context.Context, id int64, message string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{
			"status":     enum.ExportTaskStatusFailed,
			"error_msg":  capRunes(message, 500),
			"updated_at": now,
		}).Error
}

func (r *GormRepository) DeleteByUser(ctx context.Context, userID int64, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("user_id = ?", userID).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": time.Now()}).Error
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Task, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Task
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) scopedQuery(ctx context.Context, userID int64, title string, fileName string) *gorm.DB {
	db := r.db.WithContext(ctx).Model(&Task{}).Where("user_id = ?", userID).Where("is_del = ?", enum.CommonNo)
	if title = strings.TrimSpace(title); title != "" {
		db = db.Where("title LIKE ?", title+"%")
	}
	if fileName = strings.TrimSpace(fileName); fileName != "" {
		db = db.Where("file_name LIKE ?", fileName+"%")
	}
	return db
}
