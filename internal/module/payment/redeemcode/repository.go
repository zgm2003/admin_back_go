package redeemcode

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/payment/serialno"
	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

var (
	ErrRepositoryNotConfigured = errors.New("redeem code repository not configured")
	ErrRequestConflict         = errors.New("redeem code request identity conflict")
	ErrBatchNumberCollision    = errors.New("redeem code batch number collision")
	ErrCodeCollision           = errors.New("redeem code value collision")
	ErrExpiryNotFuture         = errors.New("redeem code expiry is not after creation")
	ErrVoidConflict            = errors.New("redeem code void set conflict")
	ErrUnavailable             = errors.New("redeem code unavailable")
	ErrIntegrityViolation      = errors.New("redeem code integrity violation")
	ErrExpired                 = fmt.Errorf("redeem code expired: %w", ErrUnavailable)
	ErrSourceConflict          = fmt.Errorf("redeem code source conflict: %w", ErrIntegrityViolation)
	ErrOverflow                = fmt.Errorf("redeem code wallet overflow: %w", ErrIntegrityViolation)

	errCreatorRequestRace = errors.New("redeem code creator request race")
)

const (
	duplicateKeyBatchNo        = "uk_redeem_code_batches_batch_no"
	duplicateKeyCreatorRequest = "uk_redeem_code_batches_creator_request"
	duplicateKeyCode           = "uk_redeem_codes_code"
	maxTransactionAttempts     = 3
)

type GormRepository struct {
	db                *gorm.DB
	walletParticipant wallet.TransactionParticipant
	clock             clock.Clock
}

func NewGormRepository(client *database.Client, participant wallet.TransactionParticipant, clocks ...clock.Clock) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	var repositoryClock clock.Clock = clock.SystemClock{}
	if len(clocks) > 0 && clocks[0] != nil {
		repositoryClock = clocks[0]
	}
	return &GormRepository{db: client.Gorm, walletParticipant: participant, clock: repositoryClock}
}

func (repository *GormRepository) FindBatchByRequest(ctx context.Context, createdBy int64, requestID string) (*BatchWithCodes, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	if createdBy <= 0 || !requestIDPattern.MatchString(requestID) {
		return nil, ErrIntegrityViolation
	}
	return findBatchByRequestDB(repository.db.Session(&gorm.Session{Logger: gormlogger.Discard}).WithContext(nonNilContext(ctx)), createdBy, requestID)
}

func (repository *GormRepository) CreateOrReplayBatch(ctx context.Context, record CreateBatchRecord) (*BatchWithCodes, bool, error) {
	if err := repository.ready(); err != nil {
		return nil, false, err
	}
	if err := validateCreateRecord(record); err != nil {
		return nil, false, err
	}
	ctx = nonNilContext(ctx)
	var result *BatchWithCodes
	var replayed bool
	err := repository.withTransactionRetry(ctx, func(tx *gorm.DB) error {
		attemptRecord := cloneCreateRecord(record)
		existing, err := findBatchByRequestDB(tx, attemptRecord.Batch.CreatedBy, attemptRecord.Batch.RequestID)
		if err != nil {
			return err
		}
		if existing != nil {
			if !sameRequestIdentity(existing.Batch, attemptRecord.Batch) {
				return ErrRequestConflict
			}
			result, replayed = existing, true
			return nil
		}

		createdAt := operationNow(repository.clock)
		if attemptRecord.Batch.ExpiresAt != nil && !attemptRecord.Batch.ExpiresAt.After(createdAt) {
			return ErrExpiryNotFuture
		}
		attemptRecord.Batch.CreatedAt = createdAt
		attemptRecord.Batch.UpdatedAt = createdAt
		if err := tx.Create(&attemptRecord.Batch).Error; err != nil {
			switch {
			case duplicateKeyFor(err, duplicateKeyCreatorRequest):
				return errCreatorRequestRace
			case duplicateKeyFor(err, duplicateKeyBatchNo):
				return ErrBatchNumberCollision
			default:
				return err
			}
		}
		for index := range attemptRecord.Codes {
			attemptRecord.Codes[index].ID = 0
			attemptRecord.Codes[index].BatchID = attemptRecord.Batch.ID
			attemptRecord.Codes[index].State = StateUnused
			attemptRecord.Codes[index].UsedBy = nil
			attemptRecord.Codes[index].UsedAt = nil
			attemptRecord.Codes[index].CreatedAt = createdAt
			attemptRecord.Codes[index].UpdatedAt = createdAt
		}
		codeDB := tx.Session(&gorm.Session{Logger: gormlogger.Discard})
		insert := codeDB.Create(&attemptRecord.Codes)
		if insert.Error != nil {
			if duplicateKeyFor(insert.Error, duplicateKeyCode) {
				return ErrCodeCollision
			}
			return insert.Error
		}
		if insert.RowsAffected != int64(attemptRecord.Batch.Quantity) {
			return ErrIntegrityViolation
		}
		result = &BatchWithCodes{Batch: attemptRecord.Batch, Codes: attemptRecord.Codes}
		replayed = false
		return nil
	})
	if errors.Is(err, errCreatorRequestRace) {
		existing, lookupErr := repository.FindBatchByRequest(ctx, record.Batch.CreatedBy, record.Batch.RequestID)
		if lookupErr != nil {
			return nil, false, lookupErr
		}
		if existing == nil {
			return nil, false, ErrIntegrityViolation
		}
		if !sameRequestIdentity(existing.Batch, record.Batch) {
			return nil, false, ErrRequestConflict
		}
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return result, replayed, nil
}

func (repository *GormRepository) ListCodes(ctx context.Context, query ListQuery, now time.Time) ([]CodeView, int64, error) {
	if err := repository.ready(); err != nil {
		return nil, 0, err
	}
	if query.CurrentPage <= 0 || query.PageSize <= 0 || query.PageSize > maxPageSize {
		return nil, 0, ErrIntegrityViolation
	}
	if !validRepositoryListQuery(query) {
		return nil, 0, ErrIntegrityViolation
	}
	db := repository.readQuery(nonNilContext(ctx), query, now)
	var total int64
	if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CodeView
	offset := (query.CurrentPage - 1) * query.PageSize
	if err := db.Order("rc.id DESC").Limit(query.PageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (repository *GormRepository) LookupCode(ctx context.Context, code string, now time.Time) (*CodeView, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	normalized, err := NormalizeCode(code)
	if err != nil || normalized != code {
		return nil, ErrInvalidCode
	}
	var rows []CodeView
	db := repository.readQuery(nonNilContext(ctx), ListQuery{}, now).
		Session(&gorm.Session{Logger: gormlogger.Discard}).
		Where("rc.code = ?", code).
		Order("rc.id ASC").
		Limit(2)
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, ErrIntegrityViolation
	}
	return &rows[0], nil
}

func (repository *GormRepository) ExportCodes(ctx context.Context, query ListQuery, now time.Time, limit int) ([]CodeView, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > MaxExportRows+1 {
		return nil, ErrIntegrityViolation
	}
	if !validRepositoryListQuery(query) {
		return nil, ErrIntegrityViolation
	}
	var rows []CodeView
	if err := repository.readQuery(nonNilContext(ctx), query, now).Order("rc.id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (repository *GormRepository) VoidCodes(ctx context.Context, ids []int64, now time.Time) (int, error) {
	if err := repository.ready(); err != nil {
		return 0, err
	}
	normalized, err := normalizeIDs(ids)
	if err != nil {
		return 0, err
	}
	now = now.UTC().Truncate(time.Microsecond)
	var changed int
	err = repository.withTransactionRetry(nonNilContext(ctx), func(tx *gorm.DB) error {
		var codes []Code
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", normalized).
			Order("id ASC").
			Limit(len(normalized) + 1).
			Find(&codes).Error; err != nil {
			return err
		}
		if len(codes) != len(normalized) {
			return ErrVoidConflict
		}
		toVoid := make([]int64, 0, len(codes))
		for index, code := range codes {
			if code.ID != normalized[index] {
				return ErrVoidConflict
			}
			switch code.State {
			case StateUnused:
				toVoid = append(toVoid, code.ID)
			case StateVoided:
			case StateUsed:
				return ErrVoidConflict
			default:
				return ErrIntegrityViolation
			}
		}
		changed = len(toVoid)
		if len(toVoid) == 0 {
			return nil
		}
		update := tx.Model(&Code{}).
			Where("id IN ? AND state = ?", toVoid, StateUnused).
			Updates(map[string]any{"state": StateVoided, "updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != int64(len(toVoid)) {
			return ErrVoidConflict
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

func (repository *GormRepository) Redeem(ctx context.Context, userID int64, code string) (*RedemptionFact, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	if repository.walletParticipant == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if userID <= 0 {
		return nil, ErrUnavailable
	}
	normalized, err := NormalizeCode(code)
	if err != nil || normalized != code {
		return nil, ErrUnavailable
	}
	ctx = nonNilContext(ctx)
	var fact *RedemptionFact
	var transactionNo string
	err = repository.withTransactionRetry(ctx, func(tx *gorm.DB) error {
		lockedCode, err := findCodeForUpdate(tx, code)
		if err != nil {
			return err
		}
		if lockedCode == nil {
			return ErrUnavailable
		}
		var batch Batch
		if err := tx.Where("id = ?", lockedCode.BatchID).First(&batch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIntegrityViolation
			}
			return err
		}
		if !validImmutableBatch(batch) {
			return ErrIntegrityViolation
		}

		switch lockedCode.State {
		case StateUsed:
			if lockedCode.UsedBy == nil || lockedCode.UsedAt == nil {
				return ErrIntegrityViolation
			}
			if *lockedCode.UsedBy != userID {
				return ErrUnavailable
			}
			currentWallet, transaction, findErr := repository.walletParticipant.FindRedeemCodeCreditInTx(ctx, tx, lockedCode.ID, true)
			if findErr != nil {
				return mapWalletError(findErr)
			}
			if !validWalletRedemptionFacts(currentWallet, transaction, batch, *lockedCode, userID) {
				return ErrIntegrityViolation
			}
			fact = &RedemptionFact{AmountCents: batch.AmountCents, Wallet: currentWallet, Transaction: transaction, Replayed: true}
			return nil
		case StateVoided:
			return ErrUnavailable
		case StateUnused:
			if lockedCode.UsedBy != nil || lockedCode.UsedAt != nil {
				return ErrIntegrityViolation
			}
		default:
			return ErrIntegrityViolation
		}

		decisionTime := operationNow(repository.clock)
		if batch.ExpiresAt != nil && !batch.ExpiresAt.After(decisionTime) {
			return ErrExpired
		}
		if transactionNo == "" {
			transactionNo = serialno.NewWalletTransactionNo(decisionTime)
		}
		currentWallet, transaction, creditErr := repository.walletParticipant.CreditRedeemCodeInTx(ctx, tx, wallet.RedeemCodeCreditInput{
			UserID: userID, CodeID: lockedCode.ID, AmountCents: batch.AmountCents, BatchNo: batch.BatchNo, TransactionNo: transactionNo,
		}, decisionTime)
		if creditErr != nil {
			return mapWalletError(creditErr)
		}
		if transaction != nil && transaction.TransactionNo != "" {
			transactionNo = transaction.TransactionNo
		}
		if !validWalletRedemptionFacts(currentWallet, transaction, batch, *lockedCode, userID) {
			return ErrIntegrityViolation
		}
		update := tx.Model(&Code{}).
			Where("id = ? AND state = ?", lockedCode.ID, StateUnused).
			Updates(map[string]any{"state": StateUsed, "used_by": userID, "used_at": decisionTime, "updated_at": decisionTime})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrIntegrityViolation
		}
		fact = &RedemptionFact{AmountCents: batch.AmountCents, Wallet: currentWallet, Transaction: transaction}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fact, nil
}

func (repository *GormRepository) ready() error {
	if repository == nil || repository.db == nil {
		return ErrRepositoryNotConfigured
	}
	return nil
}

func (repository *GormRepository) withTransactionRetry(ctx context.Context, operation func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < maxTransactionAttempts; attempt++ {
		err = repository.db.Session(&gorm.Session{Logger: gormlogger.Discard}).WithContext(ctx).Transaction(operation)
		if !retryableMySQLError(err) {
			return err
		}
	}
	return err
}

func findBatchByRequestDB(db *gorm.DB, createdBy int64, requestID string) (*BatchWithCodes, error) {
	var batches []Batch
	if err := db.Where("created_by = ? AND request_id = ?", createdBy, requestID).
		Order("id ASC").Limit(2).Find(&batches).Error; err != nil {
		return nil, err
	}
	if len(batches) == 0 {
		return nil, nil
	}
	if len(batches) != 1 || !validImmutableBatch(batches[0]) {
		return nil, ErrIntegrityViolation
	}
	batch := batches[0]
	var codes []Code
	if err := db.Where("batch_id = ?", batch.ID).Order("id ASC").Limit(batch.Quantity + 1).Find(&codes).Error; err != nil {
		return nil, err
	}
	if len(codes) != batch.Quantity {
		return nil, ErrIntegrityViolation
	}
	for _, code := range codes {
		if code.ID <= 0 || code.BatchID != batch.ID || code.Code == "" || !validStoredState(code) {
			return nil, ErrIntegrityViolation
		}
	}
	return &BatchWithCodes{Batch: batch, Codes: codes}, nil
}

func findCodeForUpdate(tx *gorm.DB, code string) (*Code, error) {
	var codes []Code
	err := tx.Session(&gorm.Session{Logger: gormlogger.Discard}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("code = ?", code).
		Order("id ASC").Limit(2).Find(&codes).Error
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return nil, nil
	}
	if len(codes) != 1 || codes[0].ID <= 0 || codes[0].BatchID <= 0 {
		return nil, ErrIntegrityViolation
	}
	return &codes[0], nil
}

func (repository *GormRepository) readQuery(ctx context.Context, query ListQuery, now time.Time) *gorm.DB {
	db := repository.db.Session(&gorm.Session{Logger: gormlogger.Discard}).WithContext(ctx).Table("redeem_codes AS rc").
		Select(`rc.id, rc.batch_id, rc.code, rc.state, rc.used_by, rc.used_at, rc.created_at,
            b.batch_no, b.amount_cents, b.expires_at, b.note, b.created_by,
            COALESCE(creator.username, '') AS creator_username,
            COALESCE(used_user.username, '') AS used_username,
            COALESCE(NULLIF(used_user.phone, ''), NULLIF(used_user.email, ''), used_user.username, '') AS used_account,
            COALESCE(wt.transaction_no, '') AS wallet_transaction_no`).
		Joins("JOIN redeem_code_batches AS b ON b.id = rc.batch_id").
		Joins("LEFT JOIN users AS creator ON creator.id = b.created_by AND creator.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN users AS used_user ON used_user.id = rc.used_by AND used_user.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN wallet_transactions AS wt ON wt.source_type = ? AND wt.source_id = rc.id AND wt.is_del = ?", wallet.SourceRedeemCode, enum.CommonNo)
	if query.BatchNo != "" {
		db = db.Where("b.batch_no LIKE ?", query.BatchNo+"%")
	}
	switch query.State {
	case StateUnused:
		db = db.Where("rc.state = ? AND (b.expires_at IS NULL OR b.expires_at > ?)", StateUnused, now)
	case StateExpired:
		db = db.Where("rc.state = ? AND b.expires_at IS NOT NULL AND b.expires_at <= ?", StateUnused, now)
	case StateUsed, StateVoided:
		db = db.Where("rc.state = ?", query.State)
	}
	if query.UsedBy > 0 {
		db = db.Where("rc.used_by = ?", query.UsedBy)
	}
	if query.UsedUser != "" {
		like := query.UsedUser + "%"
		db = db.Where("(used_user.username LIKE ? OR used_user.phone LIKE ? OR used_user.email LIKE ?)", like, like, like)
	}
	if query.CreatedBy > 0 {
		db = db.Where("b.created_by = ?", query.CreatedBy)
	}
	if query.Note != "" {
		db = db.Where("b.note LIKE ?", "%"+query.Note+"%")
	}
	if query.CreatedFrom != nil {
		db = db.Where("rc.created_at >= ?", *query.CreatedFrom)
	}
	if query.CreatedTo != nil {
		db = db.Where("rc.created_at < ?", *query.CreatedTo)
	}
	if query.ExpiresFrom != nil {
		db = db.Where("b.expires_at >= ?", *query.ExpiresFrom)
	}
	if query.ExpiresTo != nil {
		db = db.Where("b.expires_at < ?", *query.ExpiresTo)
	}
	return db
}

func validateCreateRecord(record CreateBatchRecord) error {
	batch := record.Batch
	if batch.ID != 0 || batch.CreatedBy <= 0 || !requestIDPattern.MatchString(batch.RequestID) ||
		batch.BatchNo == "" || batch.RequestFingerprintVersion != RequestFingerprintVersion || len(batch.RequestFingerprint) != 64 ||
		batch.AmountCents <= 0 || batch.AmountCents > MaxAmountCents || batch.Quantity <= 0 || batch.Quantity > MaxBatchQuantity ||
		len(record.Codes) != batch.Quantity {
		return ErrIntegrityViolation
	}
	seen := make(map[string]struct{}, len(record.Codes))
	for _, code := range record.Codes {
		normalized, err := NormalizeCode(code.Code)
		if err != nil || normalized != code.Code {
			return ErrIntegrityViolation
		}
		if _, exists := seen[code.Code]; exists {
			return ErrIntegrityViolation
		}
		seen[code.Code] = struct{}{}
	}
	return nil
}

func cloneCreateRecord(record CreateBatchRecord) CreateBatchRecord {
	cloned := record
	cloned.Codes = append([]Code(nil), record.Codes...)
	return cloned
}

func sameRequestIdentity(existing Batch, candidate Batch) bool {
	return existing.RequestFingerprintVersion == candidate.RequestFingerprintVersion &&
		existing.RequestFingerprint == candidate.RequestFingerprint
}

func validImmutableBatch(batch Batch) bool {
	if batch.ID <= 0 || batch.BatchNo == "" || batch.CreatedBy <= 0 || !requestIDPattern.MatchString(batch.RequestID) ||
		batch.AmountCents <= 0 || batch.AmountCents > MaxAmountCents || batch.Quantity <= 0 || batch.Quantity > MaxBatchQuantity ||
		batch.RequestFingerprintVersion == "" || !validFingerprint(batch.RequestFingerprint) || batch.CreatedAt.IsZero() {
		return false
	}
	return batch.ExpiresAt == nil || batch.ExpiresAt.After(batch.CreatedAt)
}

func validStoredState(code Code) bool {
	normalized, err := NormalizeCode(code.Code)
	if err != nil || normalized != code.Code {
		return false
	}
	switch code.State {
	case StateUnused, StateVoided:
		return code.UsedBy == nil && code.UsedAt == nil
	case StateUsed:
		return code.UsedBy != nil && code.UsedAt != nil
	default:
		return false
	}
}

func validFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validRepositoryListQuery(query ListQuery) bool {
	if query.UsedBy < 0 || query.CreatedBy < 0 {
		return false
	}
	switch query.State {
	case "", StateUnused, StateUsed, StateVoided, StateExpired:
		return true
	default:
		return false
	}
}

func validWalletRedemptionFacts(currentWallet *wallet.Wallet, transaction *wallet.Transaction, batch Batch, code Code, userID int64) bool {
	if currentWallet == nil || transaction == nil || currentWallet.ID <= 0 || currentWallet.UserID != userID || currentWallet.IsDel != enum.CommonNo {
		return false
	}
	if transaction.ID <= 0 || transaction.TransactionNo == "" || transaction.WalletID != currentWallet.ID || transaction.UserID != userID || transaction.IsDel != enum.CommonNo ||
		transaction.Direction != wallet.DirectionIn || transaction.SourceType != wallet.SourceRedeemCode || transaction.SourceID != code.ID ||
		transaction.AmountCents != batch.AmountCents || transaction.Remark != batch.BatchNo || transaction.BalanceBeforeCents < 0 {
		return false
	}
	return transaction.BalanceBeforeCents <= math.MaxInt64-transaction.AmountCents &&
		transaction.BalanceBeforeCents+transaction.AmountCents == transaction.BalanceAfterCents
}

func mapWalletError(err error) error {
	switch {
	case errors.Is(err, wallet.ErrRedeemCodeSourceExists):
		return ErrSourceConflict
	case errors.Is(err, wallet.ErrRedeemCodeBalanceOverflow), errors.Is(err, wallet.ErrRedeemCodeTotalRechargeOverflow):
		return ErrOverflow
	case errors.Is(err, wallet.ErrRedeemCodeWalletIntegrity), errors.Is(err, wallet.ErrRedeemCodeInvalidInput), errors.Is(err, wallet.ErrRedeemCodeTransactionRequired):
		return ErrIntegrityViolation
	default:
		return err
	}
}

func normalizeIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > MaxVoidCodes {
		return nil, ErrVoidConflict
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrVoidConflict
		}
		seen[id] = struct{}{}
	}
	normalized := make([]int64, 0, len(seen))
	for id := range seen {
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(left, right int) bool { return normalized[left] < normalized[right] })
	return normalized, nil
}

func duplicateKeyFor(err error, key string) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return strings.Contains(mysqlErr.Message, key)
	}
	message := err.Error()
	return strings.Contains(message, "Duplicate entry") && strings.Contains(message, key)
}

func retryableMySQLError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1213 || mysqlErr.Number == 1205)
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
