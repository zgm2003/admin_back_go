package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured = errors.New("notification task repository is not configured")
	ErrClaimInputInvalid       = errors.New("notification task claim input is invalid")
	ErrEventSinkNotConfigured  = errors.New("notification durable event sink is not configured")
)

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
	ListDueTaskIDs(ctx context.Context, now time.Time, limit int) ([]int64, error)
	ClaimNext(ctx context.Context, owner string, now time.Time, ttl time.Duration) (*Claim, error)
	ClaimByID(ctx context.Context, id int64, owner string, now time.Time, ttl time.Duration) (*Claim, error)
	Renew(ctx context.Context, id int64, owner string, token uint64, now time.Time, leaseExpiresAt time.Time) (bool, error)
	TargetUserIDs(ctx context.Context, task Task) ([]int64, error)
	InsertNotifications(ctx context.Context, rows []Notification) error
	UpdateProgress(ctx context.Context, id int64, owner string, token uint64, now time.Time, sentCount int, totalCount int) (bool, error)
	MarkSuccess(ctx context.Context, id int64, owner string, token uint64, now time.Time, sentCount int, totalCount int) (bool, error)
	MarkFailed(ctx context.Context, id int64, owner string, token uint64, now time.Time, errMsg string) (bool, error)
}

type Claim struct {
	Task           Task
	Owner          string
	Token          uint64
	LeaseExpiresAt time.Time
}

type GormRepository struct {
	db        *gorm.DB
	eventSink modulerealtime.TransactionalEventSink
}

type RepositoryOption func(*GormRepository)

func WithDurableEventSink(sink modulerealtime.TransactionalEventSink) RepositoryOption {
	return func(repository *GormRepository) {
		repository.eventSink = sink
	}
}

func NewGormRepository(client *database.Client, options ...RepositoryOption) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	repository := &GormRepository{db: client.Gorm}
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
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

func (r *GormRepository) ListDueTaskIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if now.IsZero() {
		return nil, ErrClaimInputInvalid
	}
	if limit <= 0 {
		limit = 100
	}
	var ids []int64
	err := dueTaskQuery(r.db.WithContext(ctx), now, limit).Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func dueTaskQuery(db *gorm.DB, now time.Time, limit int) *gorm.DB {
	return db.Model(&Task{}).
		Where("is_del = ?", enum.CommonNo).
		Where("((status = ? AND "+pendingDispatchSendAtCondition+") OR (status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?)))", enum.NotificationTaskStatusPending, now, enum.NotificationTaskStatusSending, now).
		Order(gorm.Expr(pendingDispatchOrder)).
		Limit(limit)
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
			Where("is_del = ?", enum.CommonNo).
			Where("((status = ? AND "+pendingDispatchSendAtCondition+") OR (status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?)))", enum.NotificationTaskStatusPending, now, enum.NotificationTaskStatusSending, now)
		if id > 0 {
			query = query.Where("id = ?", id)
		}
		var task Task
		err := query.Order(gorm.Expr(pendingDispatchOrder)).First(&task).Error
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
				"status": enum.NotificationTaskStatusSending, "claim_owner": owner,
				"claim_token": nextToken, "claim_expires_at": leaseExpiresAt, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		task.Status = enum.NotificationTaskStatusSending
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
		Where("id = ? AND status = ? AND is_del = ?", id, enum.NotificationTaskStatusSending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", owner, token, now).
		Updates(map[string]any{"claim_expires_at": leaseExpiresAt, "updated_at": now})
	return result.RowsAffected == 1, result.Error
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
	if r.eventSink == nil {
		return ErrEventSinkNotConfigured
	}
	for _, row := range rows {
		if row.SourceTaskID <= 0 || row.UserID <= 0 {
			return ErrClaimInputInvalid
		}
	}
	events := make([]*modulerealtime.Event, 0, len(rows))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for index := range rows {
			row := rows[index]
			level, levelOK := notificationLevelKey(row.Level)
			notificationType, typeOK := notificationTypeKey(row.Type)
			if !levelOK || !typeOK {
				return fmt.Errorf("%w: notification type=%d level=%d", ErrClaimInputInvalid, row.Type, row.Level)
			}
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "source_task_id"}, {Name: "user_id"}},
				DoNothing: true,
			}).Create(&row)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 || realtimePlatform(row.Platform) == "" {
				continue
			}
			occurredAt := row.CreatedAt
			if occurredAt.IsZero() {
				return fmt.Errorf("notification %d created_at was not populated", row.ID)
			}
			event, err := r.eventSink.AppendTx(ctx, tx, modulerealtime.AppendInput{
				Type:      modulerealtime.TypeNotificationCreatedV1,
				RequestID: fmt.Sprintf("notification-task-%d-%d", row.SourceTaskID, row.UserID),
				UserID:    row.UserID,
				Payload: modulerealtime.NotificationCreatedPayload{
					TaskID: row.SourceTaskID, Title: row.Title, Content: row.Content, Link: row.Link,
					Level: level, NotificationType: notificationType,
				},
				OccurredAt: occurredAt,
			})
			if err != nil {
				return err
			}
			events = append(events, event)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, event := range events {
		r.eventSink.PublishBestEffort(ctx, event)
	}
	return nil
}

func (r *GormRepository) UpdateProgress(ctx context.Context, id int64, owner string, token uint64, now time.Time, sentCount int, totalCount int) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if !validFenceInput(id, owner, token, now) || sentCount < 0 || totalCount < 0 || sentCount > totalCount {
		return false, ErrClaimInputInvalid
	}
	result := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.NotificationTaskStatusSending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", strings.TrimSpace(owner), token, now).
		Updates(map[string]any{"sent_count": sentCount, "total_count": totalCount, "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) MarkSuccess(ctx context.Context, id int64, owner string, token uint64, now time.Time, sentCount int, totalCount int) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if !validFenceInput(id, owner, token, now) || sentCount < 0 || totalCount < 0 || sentCount > totalCount {
		return false, ErrClaimInputInvalid
	}
	result := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.NotificationTaskStatusSending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", strings.TrimSpace(owner), token, now).
		Updates(map[string]any{
			"status":      enum.NotificationTaskStatusSuccess,
			"sent_count":  sentCount,
			"total_count": totalCount,
			"error_msg":   "",
			"claim_owner": nil, "claim_expires_at": nil, "updated_at": now,
		})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) MarkFailed(ctx context.Context, id int64, owner string, token uint64, now time.Time, errMsg string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if !validFenceInput(id, owner, token, now) {
		return false, ErrClaimInputInvalid
	}
	result := r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ? AND status = ? AND is_del = ?", id, enum.NotificationTaskStatusSending, enum.CommonNo).
		Where("claim_owner = ? AND claim_token = ? AND claim_expires_at > ?", strings.TrimSpace(owner), token, now).
		Updates(map[string]any{
			"status": enum.NotificationTaskStatusFailed, "error_msg": truncateRunes(errMsg, 500),
			"claim_owner": nil, "claim_expires_at": nil, "updated_at": now,
		})
	return result.RowsAffected == 1, result.Error
}

func validFenceInput(id int64, owner string, token uint64, now time.Time) bool {
	return id > 0 && strings.TrimSpace(owner) != "" && token > 0 && !now.IsZero()
}
