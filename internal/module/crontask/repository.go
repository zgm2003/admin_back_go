package crontask

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var (
	ErrRepositoryNotConfigured = errors.New("cron task repository is not configured")
	ErrTaskNotFound            = errors.New("cron task not found")
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]Task, int64, error)
	ListAll(ctx context.Context, query ListQuery) ([]Task, error)
	NameExists(ctx context.Context, name string, excludeID int64) (bool, error)
	Create(ctx context.Context, row Task) (int64, error)
	Get(ctx context.Context, id int64) (*Task, error)
	Update(ctx context.Context, id int64, row Task) error
	UpdateStatus(ctx context.Context, id int64, status int) error
	Delete(ctx context.Context, ids []int64) error
	Logs(ctx context.Context, query LogsQuery) ([]TaskLog, int64, error)
	ListEnabled(ctx context.Context) ([]Task, error)
	LogStart(ctx context.Context, task Task, now time.Time) (int64, error)
	LogEnd(ctx context.Context, logID int64, success bool, result string, errMsg string, now time.Time) error
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

func (r *GormRepository) requireDB() (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return r.db, nil
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Task, int64, error) {
	db, err := r.requireDB()
	if err != nil {
		return nil, 0, err
	}
	q := r.listQuery(db.WithContext(ctx), query)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Task
	if err := q.Order("id desc").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) ListAll(ctx context.Context, query ListQuery) ([]Task, error) {
	db, err := r.requireDB()
	if err != nil {
		return nil, err
	}
	var rows []Task
	if err := r.listQuery(db.WithContext(ctx), query).Order("id desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) listQuery(db *gorm.DB, query ListQuery) *gorm.DB {
	q := db.Model(&Task{}).Where("is_del = ?", CommonNo)
	if title := strings.TrimSpace(query.Title); title != "" {
		q = q.Where("title LIKE ?", "%"+title+"%")
	}
	if name := strings.TrimSpace(query.Name); name != "" {
		q = q.Where("name LIKE ?", name+"%")
	}
	if query.Status != nil {
		q = q.Where("status = ?", *query.Status)
	}
	return q
}

func (r *GormRepository) NameExists(ctx context.Context, name string, excludeID int64) (bool, error) {
	db, err := r.requireDB()
	if err != nil {
		return false, err
	}
	q := db.WithContext(ctx).Model(&Task{}).Where("name = ?", strings.TrimSpace(name)).Where("is_del = ?", CommonNo)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) Create(ctx context.Context, row Task) (int64, error) {
	db, err := r.requireDB()
	if err != nil {
		return 0, err
	}
	if row.IsDel == 0 {
		row.IsDel = CommonNo
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	row.UpdatedAt = row.CreatedAt
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Task, error) {
	db, err := r.requireDB()
	if err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, ErrTaskNotFound
	}
	var row Task
	err = db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) Update(ctx context.Context, id int64, row Task) error {
	db, err := r.requireDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Where("is_del = ?", CommonNo).Updates(map[string]any{
		"title": row.Title, "description": row.Description, "cron": row.Cron,
		"cron_readable": row.CronReadable, "handler": row.Handler, "status": row.Status, "updated_at": time.Now(),
	}).Error
}

func (r *GormRepository) UpdateStatus(ctx context.Context, id int64, status int) error {
	db, err := r.requireDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Where("is_del = ?", CommonNo).Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error
}

func (r *GormRepository) Delete(ctx context.Context, ids []int64) error {
	db, err := r.requireDB()
	if err != nil {
		return err
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return db.WithContext(ctx).Model(&Task{}).Where("id IN ?", ids).Where("is_del = ?", CommonNo).Updates(map[string]any{"is_del": CommonYes, "updated_at": time.Now()}).Error
}

func (r *GormRepository) Logs(ctx context.Context, query LogsQuery) ([]TaskLog, int64, error) {
	db, err := r.requireDB()
	if err != nil {
		return nil, 0, err
	}
	q := db.WithContext(ctx).Model(&TaskLog{}).Where("is_del = ?", CommonNo).Where("task_id = ?", query.TaskID)
	if query.Status != nil {
		q = q.Where("status = ?", *query.Status)
	}
	if start := strings.TrimSpace(query.StartDate); start != "" {
		q = q.Where("created_at >= ?", start)
	}
	if end := strings.TrimSpace(query.EndDate); end != "" {
		q = q.Where("created_at <= ?", end)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TaskLog
	if err := q.Order("id desc").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) ListEnabled(ctx context.Context) ([]Task, error) {
	db, err := r.requireDB()
	if err != nil {
		return nil, err
	}
	var rows []Task
	if err := db.WithContext(ctx).Where("is_del = ?", CommonNo).Where("status = ?", enum.CommonYes).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GormRepository) LogStart(ctx context.Context, task Task, now time.Time) (int64, error) {
	db, err := r.requireDB()
	if err != nil {
		return 0, err
	}
	row := TaskLog{TaskID: task.ID, TaskName: task.Name, StartTime: &now, Status: LogStatusRunning, IsDel: CommonNo, CreatedAt: now}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) LogEnd(ctx context.Context, logID int64, success bool, result string, errMsg string, now time.Time) error {
	db, err := r.requireDB()
	if err != nil {
		return err
	}
	status := LogStatusFailed
	if success {
		status = LogStatusSuccess
	}
	fields := map[string]any{"end_time": now, "status": status, "result": result, "error_msg": errMsg}
	if logID > 0 {
		var row TaskLog
		if err := db.WithContext(ctx).Where("id = ?", logID).First(&row).Error; err == nil && row.StartTime != nil {
			duration := now.Sub(*row.StartTime).Milliseconds()
			fields["duration_ms"] = duration
		}
	}
	return db.WithContext(ctx).Model(&TaskLog{}).Where("id = ?", logID).Updates(fields).Error
}
