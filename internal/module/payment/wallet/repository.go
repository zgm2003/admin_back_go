package wallet

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRepositoryNotConfigured = errors.New("wallet repository not configured")
var ErrInsufficientBalance = errors.New("wallet insufficient balance")
var ErrConsumeSourceOwnerMismatch = errors.New("wallet consume source owner mismatch")

type Repository interface {
	GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error)
	ListTransactions(ctx context.Context, query TransactionListQuery) ([]TransactionWithUser, int64, error)
	ListWalletUsers(ctx context.Context, query WalletUserListQuery) ([]WalletWithUser, int64, error)
	Consume(ctx context.Context, input ConsumeInput, now time.Time) (*Wallet, *Transaction, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return getOrCreateWallet(ctx, r.db, userID)
}

func (r *GormRepository) ListTransactions(ctx context.Context, query TransactionListQuery) ([]TransactionWithUser, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Table("wallet_transactions AS wt").
		Select("wt.*, u.username AS username, u.phone AS phone, u.email AS email").
		Joins("LEFT JOIN users AS u ON u.id = wt.user_id AND u.is_del = ?", enum.CommonNo).
		Where("wt.is_del = ?", enum.CommonNo)
	if query.UserID > 0 {
		db = db.Where("wt.user_id = ?", query.UserID)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := keyword + "%"
		db = db.Where("(wt.transaction_no LIKE ? OR wt.remark LIKE ? OR u.username LIKE ? OR u.phone LIKE ? OR u.email LIKE ?)", like, like, like, like, like)
	}
	if direction := strings.TrimSpace(query.Direction); direction != "" {
		db = db.Where("wt.direction = ?", direction)
	}
	if sourceType := strings.TrimSpace(query.SourceType); sourceType != "" {
		db = db.Where("wt.source_type = ?", sourceType)
	}
	if start := strings.TrimSpace(query.DateStart); start != "" {
		db = db.Where("wt.created_at >= ?", start)
	}
	if end := strings.TrimSpace(query.DateEnd); end != "" {
		if nextDay, ok := nextDateOnlyDay(end); ok {
			db = db.Where("wt.created_at < ?", nextDay)
		} else {
			db = db.Where("wt.created_at <= ?", end)
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TransactionWithUser
	err := db.Order("wt.id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) ListWalletUsers(ctx context.Context, query WalletUserListQuery) ([]WalletWithUser, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	_, limit, offset := normalizePage(query.CurrentPage, query.PageSize)
	db := r.db.WithContext(ctx).Table("user_wallets AS w").
		Select("w.*, u.username AS username, u.phone AS phone, u.email AS email").
		Joins("LEFT JOIN users AS u ON u.id = w.user_id AND u.is_del = ?", enum.CommonNo).
		Where("w.is_del = ?", enum.CommonNo)
	if query.UserID > 0 {
		db = db.Where("w.user_id = ?", query.UserID)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := keyword + "%"
		db = db.Where("(u.username LIKE ? OR u.phone LIKE ? OR u.email LIKE ?)", like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []WalletWithUser
	err := db.Order("w.updated_at desc, w.id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) Consume(ctx context.Context, input ConsumeInput, now time.Time) (*Wallet, *Transaction, error) {
	if r == nil || r.db == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	var resultWallet Wallet
	var resultTransaction Transaction
	var domainErr error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existingWallet, existingTransaction, err := findConsumeSource(tx, input.UserID, input.SourceID, false)
		if err != nil {
			if errors.Is(err, ErrConsumeSourceOwnerMismatch) {
				domainErr = err
				return nil
			}
			return err
		}
		if existingTransaction != nil {
			resultWallet = *existingWallet
			resultTransaction = *existingTransaction
			return nil
		}

		wallet, err := lockOrCreateWalletForUpdate(tx, input.UserID)
		if err != nil {
			return err
		}
		if wallet.BalanceCents < input.AmountCents {
			resultWallet = *wallet
			domainErr = ErrInsufficientBalance
			return nil
		}
		before := wallet.BalanceCents
		after := before - input.AmountCents
		txRow := Transaction{
			TransactionNo:      newTransactionNo(now),
			WalletID:           wallet.ID,
			UserID:             input.UserID,
			Direction:          DirectionOut,
			AmountCents:        input.AmountCents,
			BalanceBeforeCents: before,
			BalanceAfterCents:  after,
			SourceType:         SourceConsume,
			SourceID:           input.SourceID,
			Remark:             input.Remark,
			IsDel:              enum.CommonNo,
		}
		if err := tx.Create(&txRow).Error; err != nil {
			if isDuplicateKey(err) {
				// A competing transaction may have inserted the same consume source after
				// our first read. Use a locking read here so MySQL returns the latest row
				// instead of the transaction's earlier repeatable-read snapshot.
				existingWallet, existingTransaction, lookupErr := findConsumeSource(tx, input.UserID, input.SourceID, true)
				if lookupErr != nil {
					if errors.Is(lookupErr, ErrConsumeSourceOwnerMismatch) {
						domainErr = lookupErr
						return nil
					}
					return lookupErr
				}
				if existingTransaction != nil {
					resultWallet = *existingWallet
					resultTransaction = *existingTransaction
					return nil
				}
			}
			return err
		}
		if err := tx.Model(&Wallet{}).Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).Updates(map[string]any{
			"balance_cents":       after,
			"total_consume_cents": wallet.TotalConsumeCents + input.AmountCents,
		}).Error; err != nil {
			return err
		}
		wallet.BalanceCents = after
		wallet.TotalConsumeCents += input.AmountCents
		resultWallet = *wallet
		resultTransaction = txRow
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if domainErr != nil {
		if !errors.Is(domainErr, ErrInsufficientBalance) {
			return nil, nil, domainErr
		}
		return &resultWallet, nil, domainErr
	}
	return &resultWallet, &resultTransaction, nil
}

func findConsumeSource(tx *gorm.DB, userID int64, sourceID int64, lock bool) (*Wallet, *Transaction, error) {
	var existing Transaction
	query := tx.Where("source_type = ? AND source_id = ? AND is_del = ?", SourceConsume, sourceID, enum.CommonNo)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if existing.UserID != userID {
		return nil, nil, ErrConsumeSourceOwnerMismatch
	}
	var wallet Wallet
	if err := tx.Where("id = ? AND is_del = ?", existing.WalletID, enum.CommonNo).First(&wallet).Error; err != nil {
		return nil, nil, err
	}
	return &wallet, &existing, nil
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	return strings.Contains(err.Error(), "Duplicate entry")
}

func getOrCreateWallet(ctx context.Context, db *gorm.DB, userID int64) (*Wallet, error) {
	var wallet Wallet
	err := db.WithContext(ctx).Where("user_id = ? AND is_del = ?", userID, enum.CommonNo).First(&wallet).Error
	if err == nil {
		return &wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	wallet = Wallet{UserID: userID, IsDel: enum.CommonNo}
	if err := db.WithContext(ctx).Create(&wallet).Error; err != nil {
		return nil, err
	}
	return &wallet, nil
}

func lockOrCreateWalletForUpdate(tx *gorm.DB, userID int64) (*Wallet, error) {
	var wallet Wallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND is_del = ?", userID, enum.CommonNo).First(&wallet).Error
	if err == nil {
		return &wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	wallet = Wallet{UserID: userID, IsDel: enum.CommonNo}
	if err := tx.Create(&wallet).Error; err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", wallet.ID).First(&wallet).Error; err != nil {
		return nil, err
	}
	return &wallet, nil
}

func normalizePage(currentPage int, pageSize int) (int, int, int) {
	if currentPage <= 0 {
		currentPage = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return currentPage, pageSize, (currentPage - 1) * pageSize
}

func nextDateOnlyDay(value string) (string, bool) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", false
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02"), true
}
