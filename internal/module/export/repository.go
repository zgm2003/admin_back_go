package exporttask

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured = errors.New("export task repository not configured")
	ErrClaimInputInvalid       = errors.New("export task claim input is invalid")
)

type Repository interface {
	CleanExpired(ctx context.Context, now time.Time) error
	CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error)
	List(ctx context.Context, query ListQuery) ([]Task, int64, error)
	Create(ctx context.Context, row Task) (int64, error)
	ClaimNext(ctx context.Context, owner string, now time.Time, ttl time.Duration) (*Claim, error)
	ClaimByID(ctx context.Context, id int64, owner string, now time.Time, ttl time.Duration) (*Claim, error)
	Renew(ctx context.Context, id int64, owner string, token uint64, now time.Time, leaseExpiresAt time.Time) (bool, error)
	MarkSuccess(ctx context.Context, id int64, owner string, token uint64, now time.Time, result SuccessResult) (bool, error)
	MarkFailed(ctx context.Context, id int64, owner string, token uint64, now time.Time, message string) (bool, error)
	MarkPendingFailed(ctx context.Context, id int64, now time.Time, message string) (bool, error)
	DeleteByUser(ctx context.Context, userID int64, platform string, ids []int64) error
	Get(ctx context.Context, id int64) (*Task, error)
}

type Claim struct {
	Task           Task
	Owner          string
	Token          uint64
	LeaseExpiresAt time.Time
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
		Where("claim_expires_at IS NULL OR claim_expires_at <= ?", now).
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
	db := r.scopedQuery(ctx, query.UserID, query.Platform, query.Kind, query.Title, query.FileName)
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
	db := r.scopedQuery(ctx, query.UserID, query.Platform, query.Kind, query.Title, query.FileName)
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	pageQuery := db.Session(&gorm.Session{})
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if query.BeforeID > 0 {
		pageQuery = pageQuery.Where("id < ?", query.BeforeID)
	}
	offset := (query.CurrentPage - 1) * query.PageSize
	if query.BeforeID > 0 {
		offset = 0
	}
	var rows []Task
	err := pageQuery.Order("id desc").
		Limit(query.PageSize).
		Offset(offset).
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

func (r *GormRepository) ClaimNext(ctx context.Context, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	return r.claim(ctx, 0, owner, now, ttl)
}

func (r *GormRepository) ClaimByID(ctx context.Context, id int64, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	if id <= 0 {
		return nil, ErrClaimInputInvalid
	}
	return r.claim(ctx, id, owner, now, ttl)
}

func (r *GormRepository) claim(ctx context.Context, id int64, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if owner == "" || now.IsZero() || ttl <= 0 {
		return nil, ErrClaimInputInvalid
	}
	leaseExpiresAt := now.Add(ttl)
	var claim *Claim
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND is_del = ?", enum.ExportTaskStatusPending, enum.CommonNo).
			Where("claim_owner IS NULL OR claim_expires_at IS NULL OR claim_expires_at <= ?", now)
		if id > 0 {
			query = query.Where("id = ?", id)
		}
		var task Task
		err := query.Order("id asc").First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		nextToken := task.ClaimToken + 1
		if nextToken == 0 {
			return ErrClaimInputInvalid
		}
		result := tx.Model(&Task{}).
			Where("id = ? AND claim_token = ?", task.ID, task.ClaimToken).
			Updates(map[string]any{
				"claim_owner": owner, "claim_token": nextToken,
				"claim_expires_at": leaseExpiresAt, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		task.ClaimOwner = &owner
		task.ClaimToken = nextToken
		task.ClaimExpiresAt = &leaseExpiresAt
		task.UpdatedAt = now
		claim = &Claim{Task: task, Owner: owner, Token: nextToken, LeaseExpiresAt: leaseExpiresAt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

func (r *GormRepository) Renew(ctx context.Context, id int64, owner string, token uint64, now time.Time, leaseExpiresAt time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if id <= 0 || owner == "" || token == 0 || now.IsZero() || !leaseExpiresAt.After(now) {
		return false, ErrClaimInputInvalid
	}
	result := r.db.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.ExportTaskStatusPending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", owner, token, now).
		Updates(map[string]any{"claim_expires_at": leaseExpiresAt, "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) MarkSuccess(ctx context.Context, id int64, owner string, token uint64, now time.Time, result SuccessResult) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if !validFenceInput(id, owner, token, now) || strings.TrimSpace(result.ObjectKey) == "" {
		return false, ErrClaimInputInvalid
	}
	resultUpdate := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.ExportTaskStatusPending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", strings.TrimSpace(owner), token, now).
		Updates(map[string]any{
			"status":      enum.ExportTaskStatusSuccess,
			"file_name":   result.FileName,
			"file_url":    result.FileURL,
			"object_key":  result.ObjectKey,
			"file_size":   result.FileSize,
			"row_count":   result.RowCount,
			"error_msg":   "",
			"claim_owner": nil, "claim_expires_at": nil,
			"updated_at": now,
		})
	return resultUpdate.RowsAffected == 1, resultUpdate.Error
}

func (r *GormRepository) MarkFailed(ctx context.Context, id int64, owner string, token uint64, now time.Time, message string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if !validFenceInput(id, owner, token, now) {
		return false, ErrClaimInputInvalid
	}
	result := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.ExportTaskStatusPending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", strings.TrimSpace(owner), token, now).
		Updates(map[string]any{
			"status":      enum.ExportTaskStatusFailed,
			"error_msg":   capRunes(message, 500),
			"claim_owner": nil, "claim_expires_at": nil,
			"updated_at": now,
		})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) MarkPendingFailed(ctx context.Context, id int64, now time.Time, message string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if id <= 0 || now.IsZero() {
		return false, ErrClaimInputInvalid
	}
	result := r.db.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.ExportTaskStatusPending, enum.CommonNo).
		Where("claim_owner IS NULL AND claim_expires_at IS NULL").
		Updates(map[string]any{"status": enum.ExportTaskStatusFailed, "error_msg": capRunes(message, 500), "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func validFenceInput(id int64, owner string, token uint64, now time.Time) bool {
	return id > 0 && strings.TrimSpace(owner) != "" && token > 0 && !now.IsZero()
}

func (r *GormRepository) DeleteByUser(ctx context.Context, userID int64, platform string, ids []int64) error {
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
		Where("platform = ?", normalizePlatform(platform)).
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

func (r *GormRepository) scopedQuery(ctx context.Context, userID int64, platform string, kind string, title string, fileName string) *gorm.DB {
	db := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("user_id = ?", userID).
		Where("platform = ?", normalizePlatform(platform)).
		Where("is_del = ?", enum.CommonNo)
	if kind = strings.TrimSpace(kind); kind != "" {
		db = db.Where("kind = ?", kind)
	}
	if title = strings.TrimSpace(title); title != "" {
		db = db.Where("title LIKE ?", title+"%")
	}
	if fileName = strings.TrimSpace(fileName); fileName != "" {
		db = db.Where("file_name LIKE ?", fileName+"%")
	}
	return db
}
