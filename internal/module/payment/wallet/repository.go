package wallet

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRepositoryNotConfigured = errors.New("wallet repository not configured")
var ErrWalletInvariant = errors.New("wallet balance invariant violation")
var ErrRechargeCreditIntegrity = errors.New("wallet recharge credit integrity violation")
var ErrRedeemCodeTransactionRequired = errors.New("wallet redeem code outer transaction required")
var ErrRedeemCodeInvalidInput = errors.New("wallet redeem code invalid input")
var ErrRedeemCodeCreditIdentityInvalid = errors.New("wallet redeem code credit identity invalid")
var ErrRedeemCodeWalletIntegrity = errors.New("wallet redeem code wallet integrity violation")
var ErrRedeemCodeSourceExists = errors.New("wallet redeem code source already exists")
var ErrRedeemCodeBalanceOverflow = errors.New("wallet redeem code balance overflow")
var ErrRedeemCodeTotalRechargeOverflow = errors.New("wallet redeem code total recharge overflow")

const (
	duplicateKeyWalletTransactionNo     = "uk_wallet_transaction_no"
	duplicateKeyWalletTransactionSource = "uk_wallet_transaction_source"
	duplicateKeyUserWalletUser          = "uk_user_wallet_user"
	maxTransactionNoInsertAttempts      = 3
)

type Repository interface {
	GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error)
	ListTransactions(ctx context.Context, query TransactionListQuery) ([]TransactionWithUser, int64, error)
	ListWalletUsers(ctx context.Context, query WalletUserListQuery) ([]WalletWithUser, int64, error)
}

type TransactionParticipant interface {
	FindRedeemCodeCreditInTx(context.Context, *gorm.DB, int64, bool) (*Wallet, *Transaction, error)
	CreditRedeemCodeInTx(context.Context, *gorm.DB, RedeemCodeCreditInput, time.Time) (*Wallet, *Transaction, error)
}

type RechargeTransactionParticipant interface {
	FindRechargeCreditInTx(context.Context, *gorm.DB, CreditRechargeInput) (*RechargeCreditFact, error)
	CreditRechargeInTx(context.Context, *gorm.DB, CreditRechargeInput) (*RechargeCreditFact, error)
}

type PaymentParticipant interface {
	RechargeTransactionParticipant
	GetOrCreateWallet(context.Context, int64) (*Wallet, error)
	GetWallet(context.Context, int64) (*Wallet, error)
}

type HoldParticipant interface {
	ReserveHoldInTx(context.Context, *gorm.DB, ReserveHoldInput) (*Hold, error)
	TopUpHoldInTx(context.Context, *gorm.DB, TopUpHoldInput) (*Hold, error)
	CaptureHoldInTx(context.Context, *gorm.DB, CaptureHoldInput) (*Wallet, *Transaction, error)
	ReleaseHoldInTx(context.Context, *gorm.DB, ReleaseHoldInput) (*Hold, error)
}

type RetryTransactionParticipant interface {
	TransactionParticipant
	CreditRedeemCodeWithIdentityInTx(context.Context, *gorm.DB, RedeemCodeCreditInput, *RedeemCodeCreditIdentity, time.Time) (*Wallet, *Transaction, error)
}

type GormRepository struct{ db *gorm.DB }

// NewGormRepositoryFromDB binds the wallet participant to an existing process transaction source.
func NewGormRepositoryFromDB(db *gorm.DB) *GormRepository {
	if db == nil {
		return nil
	}
	return &GormRepository{db: db}
}

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

func (r *GormRepository) GetWallet(ctx context.Context, userID int64) (*Wallet, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var wallet Wallet
	err := r.db.WithContext(ctx).Where("user_id = ? AND is_del = ?", userID, enum.CommonNo).First(&wallet).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &wallet, err
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

func (r *GormRepository) FindRedeemCodeCreditInTx(ctx context.Context, tx *gorm.DB, codeID int64, lock bool) (*Wallet, *Transaction, error) {
	if r == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	if err := requireRedeemCodeTransaction(tx); err != nil {
		return nil, nil, err
	}
	if codeID <= 0 {
		return nil, nil, ErrRedeemCodeInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)

	transaction, err := findRedeemCodeSourceFact(tx, codeID, lock)
	if err != nil {
		return nil, nil, err
	}
	if transaction == nil {
		return nil, nil, nil
	}
	if transaction.IsDel != enum.CommonNo {
		return nil, nil, ErrRedeemCodeWalletIntegrity
	}

	var wallet Wallet
	walletQuery := tx.Where("id = ?", transaction.WalletID)
	if lock {
		walletQuery = walletQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := walletQuery.First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrRedeemCodeWalletIntegrity
		}
		return nil, nil, err
	}
	if !validRedeemCodeWalletFact(&wallet, transaction.UserID) || wallet.ID != transaction.WalletID || wallet.IsDel != enum.CommonNo {
		return nil, nil, ErrRedeemCodeWalletIntegrity
	}
	return &wallet, transaction, nil
}

func (r *GormRepository) CreditRedeemCodeInTx(ctx context.Context, tx *gorm.DB, input RedeemCodeCreditInput, now time.Time) (*Wallet, *Transaction, error) {
	identity := NewRedeemCodeCreditIdentity(input, now)
	return r.CreditRedeemCodeWithIdentityInTx(ctx, tx, input, identity, now)
}

func (r *GormRepository) CreditRedeemCodeWithIdentityInTx(ctx context.Context, tx *gorm.DB, input RedeemCodeCreditInput, identity *RedeemCodeCreditIdentity, now time.Time) (*Wallet, *Transaction, error) {
	if r == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	if err := requireRedeemCodeTransaction(tx); err != nil {
		return nil, nil, err
	}
	if !identity.matchesInput(input) {
		return nil, nil, ErrRedeemCodeCreditIdentityInvalid
	}
	input = normalizeRedeemCodeCreditInput(input)
	if input.UserID <= 0 || input.CodeID <= 0 || input.AmountUnits <= 0 || input.BatchNo == "" {
		return nil, nil, ErrRedeemCodeInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)

	existing, err := findRedeemCodeSourceFact(tx, input.CodeID, false)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		if existing.IsDel != enum.CommonNo {
			return nil, nil, ErrRedeemCodeWalletIntegrity
		}
		return nil, nil, ErrRedeemCodeSourceExists
	}

	wallet, err := lockOrCreateRedeemCodeWalletForUpdate(tx, input.UserID)
	if err != nil {
		return nil, nil, err
	}
	if wallet.BalanceUnits > math.MaxInt64-input.AmountUnits {
		return nil, nil, ErrRedeemCodeBalanceOverflow
	}
	if wallet.TotalRechargeUnits > math.MaxInt64-input.AmountUnits {
		return nil, nil, ErrRedeemCodeTotalRechargeOverflow
	}
	balanceAfter := wallet.BalanceUnits + input.AmountUnits
	totalRechargeAfter := wallet.TotalRechargeUnits + input.AmountUnits

	transaction := Transaction{
		TransactionNo:      identity.TransactionNo(),
		WalletID:           wallet.ID,
		UserID:             input.UserID,
		Direction:          DirectionIn,
		AmountUnits:        input.AmountUnits,
		BalanceBeforeUnits: wallet.BalanceUnits,
		BalanceAfterUnits:  balanceAfter,
		SourceType:         SourceRedeemCode,
		SourceID:           input.CodeID,
		Remark:             input.BatchNo,
		IsDel:              enum.CommonNo,
	}
	if err := createRedeemCodeTransactionWithIdentityRetry(tx, &transaction, identity); err != nil {
		if isDuplicateKeyFor(err, duplicateKeyWalletTransactionSource) {
			return nil, nil, ErrRedeemCodeSourceExists
		}
		return nil, nil, err
	}

	result := tx.Model(&Wallet{}).
		Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).
		Updates(map[string]any{"balance_units": balanceAfter, "total_recharge_units": totalRechargeAfter})
	if result.Error != nil {
		return nil, nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, nil, ErrRedeemCodeWalletIntegrity
	}
	wallet.BalanceUnits = balanceAfter
	wallet.TotalRechargeUnits = totalRechargeAfter
	return wallet, &transaction, nil
}

func requireRedeemCodeTransaction(tx *gorm.DB) error {
	if tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Error != nil {
		return ErrRedeemCodeTransactionRequired
	}
	committer, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	if !ok || committer == nil {
		return ErrRedeemCodeTransactionRequired
	}
	value := reflect.ValueOf(committer)
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil() {
		return ErrRedeemCodeTransactionRequired
	}
	return nil
}

func lockOrCreateRedeemCodeWalletForUpdate(tx *gorm.DB, userID int64) (*Wallet, error) {
	wallet, err := findRedeemCodeWalletByUserIDForUpdate(tx, userID)
	if err == nil {
		return wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	wallet = &Wallet{UserID: userID, IsDel: enum.CommonNo}
	if err := tx.Create(wallet).Error; err != nil {
		if isDuplicateKeyFor(err, duplicateKeyUserWalletUser) {
			wallet, lookupErr := findRedeemCodeWalletByUserIDForUpdate(tx, userID)
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return nil, ErrRedeemCodeWalletIntegrity
			}
			return wallet, lookupErr
		}
		return nil, err
	}

	var locked Wallet
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", wallet.ID).First(&locked).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRedeemCodeWalletIntegrity
		}
		return nil, err
	}
	if !validRedeemCodeWalletFact(&locked, userID) || locked.ID != wallet.ID || locked.IsDel != enum.CommonNo {
		return nil, ErrRedeemCodeWalletIntegrity
	}
	return &locked, nil
}

func findRedeemCodeWalletByUserIDForUpdate(tx *gorm.DB, userID int64) (*Wallet, error) {
	var wallets []Wallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		Order("id ASC").
		Limit(2).
		Find(&wallets).Error
	if err != nil {
		return nil, err
	}
	if len(wallets) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(wallets) != 1 || !validRedeemCodeWalletFact(&wallets[0], userID) || wallets[0].IsDel != enum.CommonNo {
		return nil, ErrRedeemCodeWalletIntegrity
	}
	return &wallets[0], nil
}

func findRedeemCodeSourceFact(tx *gorm.DB, codeID int64, lock bool) (*Transaction, error) {
	var transactions []Transaction
	query := tx.Where("source_type = ? AND source_id = ?", SourceRedeemCode, codeID).
		Order("id ASC").
		Limit(2)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&transactions).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(transactions) == 0 {
		return nil, nil
	}
	if len(transactions) != 1 || !validRedeemCodeTransactionFact(&transactions[0], codeID) {
		return nil, ErrRedeemCodeWalletIntegrity
	}
	return &transactions[0], nil
}

func validRedeemCodeTransactionFact(transaction *Transaction, codeID int64) bool {
	return transaction != nil &&
		transaction.ID > 0 &&
		transaction.WalletID > 0 &&
		transaction.UserID > 0 &&
		transaction.SourceType == SourceRedeemCode &&
		transaction.SourceID == codeID &&
		validRedeemCodeLifecycle(transaction.IsDel)
}

func validRedeemCodeWalletFact(wallet *Wallet, userID int64) bool {
	return wallet != nil &&
		wallet.ID > 0 &&
		wallet.UserID > 0 &&
		wallet.UserID == userID &&
		validRedeemCodeLifecycle(wallet.IsDel)
}

func validRedeemCodeLifecycle(isDel int) bool {
	return isDel == enum.CommonYes || isDel == enum.CommonNo
}

func createTransactionWithNumberRetry(tx *gorm.DB, row *Transaction, now time.Time) error {
	var err error
	for attempt := 0; attempt < maxTransactionNoInsertAttempts; attempt++ {
		if attempt > 0 {
			row.TransactionNo = newTransactionNo(now)
		}
		err = tx.Create(row).Error
		if err == nil {
			return nil
		}
		if !isDuplicateKeyFor(err, duplicateKeyWalletTransactionNo) {
			return err
		}
	}
	return err
}

func createRedeemCodeTransactionWithIdentityRetry(tx *gorm.DB, row *Transaction, identity *RedeemCodeCreditIdentity) error {
	var err error
	for _, transactionNo := range identity.transactionNos {
		row.TransactionNo = transactionNo
		err = tx.Create(row).Error
		if err == nil {
			return nil
		}
		if !isDuplicateKeyFor(err, duplicateKeyWalletTransactionNo) {
			return err
		}
	}
	return err
}

func isDuplicateKeyFor(err error, key string) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return strings.Contains(mysqlErr.Message, key)
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") && strings.Contains(msg, key)
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
		if isDuplicateKeyFor(err, duplicateKeyUserWalletUser) {
			return getWalletByUserID(ctx, db, userID, false)
		}
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
		if isDuplicateKeyFor(err, duplicateKeyUserWalletUser) {
			ctx := tx.Statement.Context
			if ctx == nil {
				ctx = context.Background()
			}
			return getWalletByUserID(ctx, tx, userID, true)
		}
		return nil, err
	}
	var locked Wallet
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", wallet.ID).First(&locked).Error; err != nil {
		return nil, err
	}
	return &locked, nil
}

func getWalletByUserID(ctx context.Context, db *gorm.DB, userID int64, lock bool) (*Wallet, error) {
	var wallet Wallet
	query := db.WithContext(ctx).Where("user_id = ? AND is_del = ?", userID, enum.CommonNo)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&wallet).Error; err != nil {
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
