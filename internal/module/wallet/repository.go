package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Transactions(ctx context.Context, query TransactionListQuery) ([]TransactionRow, int64, error)
	CreateAdjustment(ctx context.Context, input AdjustmentMutation) (*AdjustmentResult, error)
}

type GormRepository struct {
	db *gorm.DB
}

type adjustmentExt struct {
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
	Delta          int    `json:"delta"`
	UserID         int64  `json:"user_id"`
	OperatorID     int64  `json:"operator_id"`
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
	db := r.baseWalletQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ListRow
	err := db.Select(`
			w.id,
			w.user_id,
			u.username AS user_name,
			u.email AS user_email,
			w.balance,
			w.frozen,
			w.total_recharge,
			w.total_consume,
			w.created_at
		`).
		Order("w.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) Transactions(ctx context.Context, query TransactionListQuery) ([]TransactionRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.baseTransactionQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []TransactionRow
	err := db.Select(`
			wt.id,
			wt.user_id,
			u.username AS user_name,
			u.email AS user_email,
			wt.biz_action_no,
			wt.type,
			wt.available_delta,
			wt.frozen_delta,
			wt.balance_before,
			wt.balance_after,
			wt.order_no,
			wt.title,
			wt.remark,
			wt.created_at
		`).
		Order("wt.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) CreateAdjustment(ctx context.Context, input AdjustmentMutation) (*AdjustmentResult, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var result *AdjustmentResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res, err := createAdjustmentInTx(ctx, tx, input)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		if isDuplicateKey(err) {
			return findExistingAdjustmentWithDB(ctx, r.db, input)
		}
		return nil, err
	}
	return result, nil
}

func (r *GormRepository) baseWalletQuery(ctx context.Context, query ListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("user_wallets AS w").
		Joins("LEFT JOIN users AS u ON u.id = w.user_id AND u.is_del = ?", enum.CommonNo).
		Where("w.is_del = ?", enum.CommonNo)
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("w.user_id = ?", *query.UserID)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("w.created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("w.created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}

func createAdjustmentInTx(ctx context.Context, tx *gorm.DB, input AdjustmentMutation) (*AdjustmentResult, error) {
	if existing, err := findAdjustmentByBizActionNo(ctx, tx, input.BizActionNo); err != nil || existing != nil {
		if err != nil {
			return nil, err
		}
		return resultFromExistingAdjustment(existing, input)
	}

	var user walletUser
	err := tx.WithContext(ctx).Where("id = ?", input.UserID).Where("is_del = ?", enum.CommonNo).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	walletRow, err := lockOrCreateWallet(ctx, tx, input.UserID)
	if err != nil {
		return nil, err
	}
	if input.Delta < 0 && walletRow.Balance < -input.Delta {
		return nil, ErrInsufficientBalance
	}

	before := walletRow.Balance
	after := before + input.Delta
	update := tx.WithContext(ctx).Model(&UserWallet{}).
		Where("id = ?", walletRow.ID).
		Where("version = ?", walletRow.Version).
		Where("is_del = ?", enum.CommonNo)
	if input.Delta < 0 {
		update = update.Where("balance >= ?", -input.Delta)
	}
	updated := update.Updates(map[string]any{
		"balance":    after,
		"version":    gorm.Expr("version + 1"),
		"updated_at": time.Now(),
	})
	if updated.Error != nil {
		return nil, updated.Error
	}
	if updated.RowsAffected == 0 {
		return nil, ErrInsufficientBalance
	}

	extValue, err := json.Marshal(adjustmentExt{
		IdempotencyKey: input.IdempotencyKey,
		Reason:         input.Reason,
		Delta:          input.Delta,
		UserID:         input.UserID,
		OperatorID:     input.OperatorID,
	})
	if err != nil {
		return nil, err
	}
	transaction := WalletTransaction{
		BizActionNo:    input.BizActionNo,
		UserID:         input.UserID,
		WalletID:       walletRow.ID,
		Type:           enum.WalletTypeAdjust,
		AvailableDelta: input.Delta,
		FrozenDelta:    0,
		BalanceBefore:  before,
		BalanceAfter:   after,
		FrozenBefore:   walletRow.Frozen,
		FrozenAfter:    walletRow.Frozen,
		OrderID:        0,
		OrderNo:        "",
		SourceType:     enum.WalletSourceManual,
		SourceID:       0,
		Title:          "系统调账",
		Remark:         input.Reason,
		OperatorID:     input.OperatorID,
		Ext:            string(extValue),
		IsDel:          enum.CommonNo,
	}
	if err := tx.WithContext(ctx).Create(&transaction).Error; err != nil {
		return nil, err
	}
	return &AdjustmentResult{
		TransactionID: transaction.ID,
		BizActionNo:   transaction.BizActionNo,
		BalanceBefore: transaction.BalanceBefore,
		BalanceAfter:  transaction.BalanceAfter,
	}, nil
}

func lockOrCreateWallet(ctx context.Context, tx *gorm.DB, userID int64) (*UserWallet, error) {
	var walletRow UserWallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		Where("is_del = ?", enum.CommonNo).
		First(&walletRow).Error
	if err == nil {
		return &walletRow, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	walletRow = UserWallet{UserID: userID, IsDel: enum.CommonNo}
	if err := tx.WithContext(ctx).Create(&walletRow).Error; err != nil {
		return nil, err
	}
	return &walletRow, nil
}

func findExistingAdjustmentWithDB(ctx context.Context, db *gorm.DB, input AdjustmentMutation) (*AdjustmentResult, error) {
	existing, err := findAdjustmentByBizActionNo(ctx, db, input.BizActionNo)
	if err != nil {
		return nil, err
	}
	return resultFromExistingAdjustment(existing, input)
}

func findAdjustmentByBizActionNo(ctx context.Context, db *gorm.DB, bizActionNo string) (*WalletTransaction, error) {
	var existing WalletTransaction
	err := db.WithContext(ctx).
		Where("biz_action_no = ?", bizActionNo).
		Where("is_del = ?", enum.CommonNo).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func resultFromExistingAdjustment(existing *WalletTransaction, input AdjustmentMutation) (*AdjustmentResult, error) {
	if existing == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if !existingAdjustmentMatches(existing, input) {
		return nil, ErrAdjustmentConflict
	}
	return &AdjustmentResult{
		TransactionID: existing.ID,
		BizActionNo:   existing.BizActionNo,
		BalanceBefore: existing.BalanceBefore,
		BalanceAfter:  existing.BalanceAfter,
	}, nil
}

func existingAdjustmentMatches(existing *WalletTransaction, input AdjustmentMutation) bool {
	if existing.BizActionNo != input.BizActionNo ||
		existing.UserID != input.UserID ||
		existing.AvailableDelta != input.Delta ||
		strings.TrimSpace(existing.Remark) != input.Reason ||
		existing.OperatorID != input.OperatorID {
		return false
	}
	var ext adjustmentExt
	if strings.TrimSpace(existing.Ext) == "" {
		return false
	}
	if err := json.Unmarshal([]byte(existing.Ext), &ext); err != nil {
		return false
	}
	return ext.IdempotencyKey == input.IdempotencyKey &&
		ext.Reason == input.Reason &&
		ext.Delta == input.Delta &&
		ext.UserID == input.UserID &&
		ext.OperatorID == input.OperatorID
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func (r *GormRepository) baseTransactionQuery(ctx context.Context, query TransactionListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("wallet_transactions AS wt").
		Joins("LEFT JOIN users AS u ON u.id = wt.user_id AND u.is_del = ?", enum.CommonNo).
		Where("wt.is_del = ?", enum.CommonNo)
	if query.UserID != nil && *query.UserID > 0 {
		db = db.Where("wt.user_id = ?", *query.UserID)
	}
	if query.Type != nil {
		db = db.Where("wt.type = ?", *query.Type)
	}
	if strings.TrimSpace(query.StartDate) != "" {
		db = db.Where("wt.created_at >= ?", strings.TrimSpace(query.StartDate)+" 00:00:00")
	}
	if strings.TrimSpace(query.EndDate) != "" {
		db = db.Where("wt.created_at <= ?", strings.TrimSpace(query.EndDate)+" 23:59:59")
	}
	return db
}
