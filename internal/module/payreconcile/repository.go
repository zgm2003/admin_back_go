package payreconcile

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

type Repository interface {
	ActivePayChannels(ctx context.Context) ([]ChannelSummary, error)
	FindReconcileTask(ctx context.Context, channelID int64, date time.Time, billType int) (*Task, error)
	CreateReconcileTask(ctx context.Context, task Task) error
	ListPendingTasks(ctx context.Context, limit int) ([]Task, error)
	GetTaskForUpdate(ctx context.Context, id int64) (*Task, error)
	MarkTaskStatus(ctx context.Context, id int64, status int, fields map[string]any) error
	ListSuccessfulTransactionsForBill(ctx context.Context, channelID int64, date time.Time) ([]BillTransactionRow, error)
	WithTx(ctx context.Context, fn func(Repository) error) error
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Detail(ctx context.Context, id int64) (*Task, error)
	UpdateRetry(ctx context.Context, id int64, fields map[string]any) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ActivePayChannels(ctx context.Context) ([]ChannelSummary, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ChannelSummary
	err := r.db.WithContext(ctx).
		Table("pay_channel").
		Select("id, name, channel").
		Where("status = ?", enum.CommonYes).
		Where("is_del = ?", enum.CommonNo).
		Order("sort asc, id asc").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx})
	})
}

func (r *GormRepository) FindReconcileTask(ctx context.Context, channelID int64, date time.Time, billType int) (*Task, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Task
	err := r.db.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Where("reconcile_date = ?", date.Format(dateLayout)).
		Where("bill_type = ?", billType).
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

func (r *GormRepository) CreateReconcileTask(ctx context.Context, task Task) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Create(&task).Error
}

func (r *GormRepository) ListPendingTasks(ctx context.Context, limit int) ([]Task, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = defaultReconcileExecuteLimit
	}
	if limit > maxReconcileExecuteLimit {
		limit = maxReconcileExecuteLimit
	}
	var rows []Task
	err := r.db.WithContext(ctx).
		Where("status = ?", ReconcilePending).
		Where("is_del = ?", enum.CommonNo).
		Order("reconcile_date asc").
		Order("id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetTaskForUpdate(ctx context.Context, id int64) (*Task, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Task
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
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

func (r *GormRepository) MarkTaskStatus(ctx context.Context, id int64, status int, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	values := map[string]any{"status": status}
	for key, value := range fields {
		values[key] = value
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(values).Error
}

func (r *GormRepository) ListSuccessfulTransactionsForBill(ctx context.Context, channelID int64, date time.Time) ([]BillTransactionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)
	var rows []BillTransactionRow
	err := r.db.WithContext(ctx).
		Table("pay_transactions").
		Select("transaction_no, order_no, trade_no, amount, status, paid_at").
		Where("channel_id = ?", channelID).
		Where("status = ?", enum.PayTxnSuccess).
		Where("is_del = ?", enum.CommonNo).
		Where("paid_at >= ? AND paid_at < ?", start, end).
		Order("paid_at asc").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.baseListQuery(ctx, query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ListRow
	err := db.Select(`
		id,
		reconcile_date,
		channel,
		channel_id,
		bill_type,
		status,
		platform_count,
		platform_amount,
		local_count,
		local_amount,
		diff_count,
		diff_amount,
		started_at,
		finished_at,
		created_at
	`).
		Order("reconcile_date desc").
		Order("id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	return rows, total, err
}

func (r *GormRepository) Detail(ctx context.Context, id int64) (*Task, error) {
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

func (r *GormRepository) UpdateRetry(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) baseListQuery(ctx context.Context, query ListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).Table("pay_reconcile_tasks").Where("is_del = ?", enum.CommonNo)
	if query.Channel != nil {
		db = db.Where("channel = ?", *query.Channel)
	}
	if query.BillType != nil {
		db = db.Where("bill_type = ?", *query.BillType)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("reconcile_date >= ?", strings.TrimSpace(query.StartDate))
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("reconcile_date <= ?", strings.TrimSpace(query.EndDate))
	}
	return db
}
