package wallet

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"admin_back_go/internal/shared/enum"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrHoldTransactionRequired = errors.New("wallet hold outer transaction required")
	ErrHoldInvalidInput        = errors.New("wallet hold invalid input")
	ErrHoldInsufficient        = errors.New("wallet hold insufficient available balance")
	ErrHoldUnderflow           = errors.New("wallet hold underflow")
	ErrHoldOwnerMismatch       = errors.New("wallet hold owner mismatch")
	ErrHoldSummaryInvalid      = errors.New("wallet hold source summary invalid")
	ErrHoldIntegrity           = errors.New("wallet hold integrity violation")
)

func (r *GormRepository) ReserveHoldInTx(ctx context.Context, tx *gorm.DB, in ReserveHoldInput) (*Hold, error) {
	return r.reserveHold(ctx, tx, in.UserID, in.RunID, in.AmountUnits)
}

func (r *GormRepository) TopUpHoldInTx(ctx context.Context, tx *gorm.DB, in TopUpHoldInput) (*Hold, error) {
	return r.reserveHold(ctx, tx, in.UserID, in.RunID, in.AmountUnits)
}

func (r *GormRepository) reserveHold(ctx context.Context, tx *gorm.DB, userID, runID, target int64) (*Hold, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := requireHoldTransaction(tx); err != nil {
		return nil, err
	}
	if userID <= 0 || runID <= 0 || target <= 0 {
		return nil, ErrHoldInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)
	wallet, err := lockOrCreateWalletForUpdateUnits(tx, userID)
	if err != nil {
		return nil, err
	}
	if wallet.HeldUnits < 0 || wallet.BalanceUnits < wallet.HeldUnits {
		return nil, ErrHoldIntegrity
	}
	if err := assertActiveHoldTotal(tx, wallet.ID, wallet.HeldUnits); err != nil {
		return nil, err
	}
	hold, err := lockHoldByRun(tx, runID)
	if err != nil {
		return nil, err
	}
	if hold != nil {
		if hold.UserID != userID || hold.WalletID != wallet.ID {
			return nil, ErrHoldOwnerMismatch
		}
		if hold.Status != HoldActive {
			switch hold.Status {
			case HoldCaptured:
				if _, err := validateCapturedHoldFact(tx, hold, wallet, userID); err != nil {
					return nil, err
				}
			case HoldReleased:
				if err := validateReleasedHoldFact(tx, hold, wallet, userID); err != nil {
					return nil, err
				}
			}
			return nil, ErrHoldIntegrity
		}
		if err := validateActiveHoldFact(hold, wallet, userID); err != nil {
			return nil, err
		}
		if target <= hold.HeldUnits {
			return hold, nil
		}
	} else {
		hold = &Hold{WalletID: wallet.ID, UserID: userID, RunID: runID, Status: HoldActive}
	}
	delta := target
	if hold.ID > 0 {
		delta = target - hold.HeldUnits
	}
	if delta <= 0 {
		return hold, nil
	}
	if wallet.BalanceUnits < wallet.HeldUnits || delta > wallet.BalanceUnits-wallet.HeldUnits {
		return nil, ErrHoldInsufficient
	}
	if wallet.HeldUnits > math.MaxInt64-delta {
		return nil, ErrHoldIntegrity
	}
	if hold.ID == 0 {
		hold.HeldUnits = target
		result := tx.Create(hold)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrHoldIntegrity
		}
	} else {
		hold.HeldUnits += delta
		result := tx.Model(&Hold{}).Where("id = ? AND status = ?", hold.ID, HoldActive).Updates(map[string]any{"held_units": hold.HeldUnits})
		if result.Error != nil || result.RowsAffected != 1 {
			if result.Error != nil {
				return nil, result.Error
			}
			return nil, ErrHoldIntegrity
		}
	}
	wallet.HeldUnits += delta
	result := tx.Model(&Wallet{}).Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).Updates(map[string]any{"held_units": wallet.HeldUnits})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, ErrHoldIntegrity
	}
	if err := assertActiveHoldTotal(tx, wallet.ID, wallet.HeldUnits); err != nil {
		return nil, err
	}
	return hold, nil
}

func (r *GormRepository) CaptureHoldInTx(ctx context.Context, tx *gorm.DB, in CaptureHoldInput) (*Wallet, *Transaction, error) {
	if r == nil || r.db == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	if err := requireHoldTransaction(tx); err != nil {
		return nil, nil, err
	}
	if in.UserID <= 0 || in.RunID <= 0 || in.ActualUnits <= 0 {
		return nil, nil, ErrHoldInvalidInput
	}
	summary := strings.TrimSpace(in.SourceSummary)
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)
	wallet, err := lockWalletForUpdateUnits(tx, in.UserID)
	if err != nil {
		return nil, nil, err
	}
	if wallet.BalanceUnits < wallet.HeldUnits {
		return nil, nil, ErrHoldIntegrity
	}
	if err := assertActiveHoldTotal(tx, wallet.ID, wallet.HeldUnits); err != nil {
		return nil, nil, err
	}
	hold, err := lockHoldByRun(tx, in.RunID)
	if err != nil {
		return nil, nil, err
	}
	if hold == nil {
		return nil, nil, gorm.ErrRecordNotFound
	}
	if hold.UserID != in.UserID || hold.WalletID != wallet.ID {
		return nil, nil, ErrHoldOwnerMismatch
	}
	if hold.Status != HoldActive {
		if hold.Status != HoldCaptured || in.ActualUnits != hold.CapturedUnits {
			return nil, nil, ErrHoldIntegrity
		}
		ledger, terminalErr := validateCapturedHoldFact(tx, hold, wallet, in.UserID)
		if terminalErr != nil {
			return nil, nil, terminalErr
		}
		return wallet, ledger, nil
	}
	if err := validateActiveHoldFact(hold, wallet, in.UserID); err != nil {
		return nil, nil, err
	}
	if err := validateHoldSummary(summary); err != nil {
		return nil, nil, err
	}
	if in.ActualUnits > hold.HeldUnits || wallet.HeldUnits < hold.HeldUnits || wallet.BalanceUnits < in.ActualUnits {
		return nil, nil, ErrHoldUnderflow
	}
	remaining := wallet.HeldUnits - hold.HeldUnits
	balanceAfter := wallet.BalanceUnits - in.ActualUnits
	if in.ActualUnits > 0 && wallet.TotalConsumeUnits > math.MaxInt64-in.ActualUnits {
		return nil, nil, ErrHoldIntegrity
	}
	var ledger *Transaction
	if in.ActualUnits > 0 {
		row := &Transaction{TransactionNo: newTransactionNo(time.Now()), WalletID: wallet.ID, UserID: in.UserID, Direction: DirectionOut, AmountUnits: in.ActualUnits, BalanceBeforeUnits: wallet.BalanceUnits, BalanceAfterUnits: balanceAfter, SourceType: SourceAIGenerate, SourceID: in.RunID, Remark: summary, IsDel: enum.CommonNo}
		if err := createTransactionWithNumberRetry(tx, row, time.Now()); err != nil {
			return nil, nil, err
		}
		ledger = row
	}
	result := tx.Model(&Wallet{}).Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).Updates(map[string]any{"balance_units": balanceAfter, "total_consume_units": wallet.TotalConsumeUnits + in.ActualUnits, "held_units": remaining})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return nil, nil, result.Error
		}
		return nil, nil, ErrHoldIntegrity
	}
	result = tx.Model(&Hold{}).Where("id = ? AND status = ?", hold.ID, HoldActive).Updates(map[string]any{"held_units": 0, "captured_units": in.ActualUnits, "status": HoldCaptured})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return nil, nil, result.Error
		}
		return nil, nil, ErrHoldIntegrity
	}
	wallet.BalanceUnits, wallet.TotalConsumeUnits, wallet.HeldUnits = balanceAfter, wallet.TotalConsumeUnits+in.ActualUnits, remaining
	hold.HeldUnits, hold.CapturedUnits, hold.Status = 0, in.ActualUnits, HoldCaptured
	if err := assertActiveHoldTotal(tx, wallet.ID, wallet.HeldUnits); err != nil {
		return nil, nil, err
	}
	return wallet, ledger, nil
}

func validateHoldSummary(summary string) error {
	if summary == "" || utf8.RuneCountInString(summary) > 255 || strings.IndexFunc(summary, unicode.IsControl) >= 0 {
		return ErrHoldSummaryInvalid
	}
	return nil
}

func (r *GormRepository) ReleaseHoldInTx(ctx context.Context, tx *gorm.DB, in ReleaseHoldInput) (*Hold, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := requireHoldTransaction(tx); err != nil {
		return nil, err
	}
	if in.UserID <= 0 || in.RunID <= 0 {
		return nil, ErrHoldInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)
	wallet, err := lockWalletForUpdateUnits(tx, in.UserID)
	if err != nil {
		return nil, err
	}
	if wallet.BalanceUnits < wallet.HeldUnits {
		return nil, ErrHoldIntegrity
	}
	if err := assertActiveHoldTotal(tx, wallet.ID, wallet.HeldUnits); err != nil {
		return nil, err
	}
	hold, err := lockHoldByRun(tx, in.RunID)
	if err != nil {
		return nil, err
	}
	if hold == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if hold.UserID != in.UserID || hold.WalletID != wallet.ID {
		return nil, ErrHoldOwnerMismatch
	}
	if hold.Status != HoldActive {
		if hold.Status != HoldReleased {
			return nil, ErrHoldIntegrity
		}
		if err := validateReleasedHoldFact(tx, hold, wallet, in.UserID); err != nil {
			return nil, err
		}
		return hold, nil
	}
	if err := validateActiveHoldFact(hold, wallet, in.UserID); err != nil {
		return nil, err
	}
	if wallet.HeldUnits < hold.HeldUnits {
		return nil, ErrHoldUnderflow
	}
	wallet.HeldUnits -= hold.HeldUnits
	result := tx.Model(&Wallet{}).Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).Updates(map[string]any{"held_units": wallet.HeldUnits})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, ErrHoldIntegrity
	}
	result = tx.Model(&Hold{}).Where("id = ? AND status = ?", hold.ID, HoldActive).Updates(map[string]any{"held_units": 0, "captured_units": 0, "status": HoldReleased})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, ErrHoldIntegrity
	}
	hold.HeldUnits, hold.Status = 0, HoldReleased
	if err := assertActiveHoldTotal(tx, wallet.ID, wallet.HeldUnits); err != nil {
		return nil, err
	}
	return hold, nil
}

func (r *GormRepository) FindRechargeCreditInTx(ctx context.Context, tx *gorm.DB, in CreditRechargeInput) (*RechargeCreditFact, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := requireHoldTransaction(tx); err != nil {
		return nil, err
	}
	if in.UserID <= 0 || in.RechargeID <= 0 || in.AmountUnits <= 0 {
		return nil, ErrHoldInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)
	wallet, err := lockWalletForUpdateUnits(tx, in.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRechargeCreditIntegrity
		}
		return nil, err
	}
	existing, err := findRechargeCreditTransactionForUpdate(tx, in.RechargeID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrRechargeCreditIntegrity
	}
	if err := validateRechargeCreditFact(wallet, existing, in); err != nil {
		return nil, err
	}
	return &RechargeCreditFact{Wallet: wallet, Transaction: existing, Disposition: RechargeCreditReplayed}, nil
}

func (r *GormRepository) CreditRechargeInTx(ctx context.Context, tx *gorm.DB, in CreditRechargeInput) (*RechargeCreditFact, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := requireHoldTransaction(tx); err != nil {
		return nil, err
	}
	if in.UserID <= 0 || in.RechargeID <= 0 || in.AmountUnits <= 0 {
		return nil, ErrHoldInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)
	wallet, err := lockOrCreateWalletForUpdateUnits(tx, in.UserID)
	if err != nil {
		return nil, err
	}
	existing, err := findRechargeCreditTransactionForUpdate(tx, in.RechargeID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if err := validateRechargeCreditFact(wallet, existing, in); err != nil {
			return nil, err
		}
		return &RechargeCreditFact{Wallet: wallet, Transaction: existing, Disposition: RechargeCreditReplayed}, nil
	}
	if wallet.BalanceUnits > math.MaxInt64-in.AmountUnits || wallet.TotalRechargeUnits > math.MaxInt64-in.AmountUnits {
		return nil, ErrHoldIntegrity
	}
	after := wallet.BalanceUnits + in.AmountUnits
	row := &Transaction{TransactionNo: newTransactionNo(time.Now()), WalletID: wallet.ID, UserID: in.UserID, Direction: DirectionIn, AmountUnits: in.AmountUnits, BalanceBeforeUnits: wallet.BalanceUnits, BalanceAfterUnits: after, SourceType: SourceRecharge, SourceID: in.RechargeID, Remark: strings.TrimSpace(in.Remark), IsDel: enum.CommonNo}
	if err := createTransactionWithNumberRetry(tx, row, time.Now()); err != nil {
		return nil, err
	}
	result := tx.Model(&Wallet{}).Where("id = ? AND is_del = ?", wallet.ID, enum.CommonNo).Updates(map[string]any{"balance_units": after, "total_recharge_units": wallet.TotalRechargeUnits + in.AmountUnits})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrHoldIntegrity
	}
	wallet.BalanceUnits, wallet.TotalRechargeUnits = after, wallet.TotalRechargeUnits+in.AmountUnits
	return &RechargeCreditFact{Wallet: wallet, Transaction: row, Disposition: RechargeCreditCreated}, nil
}

func findRechargeCreditTransactionForUpdate(tx *gorm.DB, rechargeID int64) (*Transaction, error) {
	var transactions []Transaction
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("source_type = ? AND source_id = ?", SourceRecharge, rechargeID).
		Order("id ASC").
		Limit(2).
		Find(&transactions).Error; err != nil {
		return nil, err
	}
	if len(transactions) == 0 {
		return nil, nil
	}
	if len(transactions) != 1 {
		return nil, ErrRechargeCreditIntegrity
	}
	return &transactions[0], nil
}

func validateRechargeCreditFact(wallet *Wallet, transaction *Transaction, in CreditRechargeInput) error {
	if wallet == nil || transaction == nil || wallet.ID <= 0 || wallet.UserID != in.UserID || wallet.IsDel != enum.CommonNo ||
		wallet.BalanceUnits < 0 || wallet.TotalRechargeUnits < in.AmountUnits || wallet.HeldUnits < 0 || wallet.HeldUnits > wallet.BalanceUnits ||
		transaction.ID <= 0 || strings.TrimSpace(transaction.TransactionNo) == "" || transaction.IsDel != enum.CommonNo ||
		transaction.WalletID != wallet.ID || transaction.UserID != in.UserID || transaction.Direction != DirectionIn ||
		transaction.AmountUnits != in.AmountUnits || transaction.SourceType != SourceRecharge || transaction.SourceID != in.RechargeID ||
		transaction.BalanceBeforeUnits < 0 || transaction.BalanceBeforeUnits > math.MaxInt64-in.AmountUnits ||
		transaction.BalanceAfterUnits != transaction.BalanceBeforeUnits+in.AmountUnits {
		return ErrRechargeCreditIntegrity
	}
	return nil
}

func requireHoldTransaction(tx *gorm.DB) error {
	if tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Error != nil {
		return ErrHoldTransactionRequired
	}
	committer, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	if !ok || committer == nil {
		return ErrHoldTransactionRequired
	}
	value := reflect.ValueOf(committer)
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil() {
		return ErrHoldTransactionRequired
	}
	return nil
}

func lockOrCreateWalletForUpdateUnits(tx *gorm.DB, userID int64) (*Wallet, error) {
	return lockOrCreateWalletForUpdate(tx, userID)
}

func lockWalletForUpdateUnits(tx *gorm.DB, userID int64) (*Wallet, error) {
	var wallet Wallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND is_del = ?", userID, enum.CommonNo).First(&wallet).Error
	return &wallet, err
}

func lockHoldByRun(tx *gorm.DB, runID int64) (*Hold, error) {
	var hold Hold
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).First(&hold).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &hold, nil
}

func assertActiveHoldTotal(tx *gorm.DB, walletID, heldUnits int64) error {
	var row struct{ Total int64 }
	if err := tx.Raw("SELECT COALESCE(SUM(held_units), 0) AS total FROM wallet_holds WHERE wallet_id = ? AND status = ? FOR UPDATE", walletID, HoldActive).Scan(&row).Error; err != nil {
		return err
	}
	if row.Total != heldUnits {
		return ErrHoldIntegrity
	}
	return nil
}

func terminalAIGenerateTransactions(tx *gorm.DB, hold *Hold) ([]Transaction, error) {
	var transactions []Transaction
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_type = ? AND source_id = ?", SourceAIGenerate, hold.RunID).Order("id ASC").Limit(2).Find(&transactions).Error; err != nil {
		return nil, err
	}
	return transactions, nil
}

func validateActiveHoldFact(hold *Hold, wallet *Wallet, userID int64) error {
	if hold == nil || wallet == nil || hold.UserID != userID || hold.WalletID != wallet.ID {
		return ErrHoldOwnerMismatch
	}
	if hold.Status != HoldActive || hold.HeldUnits <= 0 || hold.CapturedUnits != 0 {
		return ErrHoldIntegrity
	}
	return nil
}

func validateCapturedHoldFact(tx *gorm.DB, hold *Hold, wallet *Wallet, userID int64) (*Transaction, error) {
	if hold == nil || wallet == nil || hold.UserID != userID || hold.WalletID != wallet.ID {
		return nil, ErrHoldOwnerMismatch
	}
	transactions, err := terminalAIGenerateTransactions(tx, hold)
	if err != nil {
		return nil, err
	}
	if hold.HeldUnits != 0 || hold.CapturedUnits <= 0 {
		return nil, ErrHoldIntegrity
	}
	if len(transactions) != 1 {
		return nil, ErrHoldIntegrity
	}
	transaction := transactions[0]
	if transaction.IsDel != enum.CommonNo || transaction.WalletID != wallet.ID || transaction.UserID != userID || transaction.Direction != DirectionOut || transaction.SourceType != SourceAIGenerate || transaction.SourceID != hold.RunID || transaction.AmountUnits != hold.CapturedUnits || transaction.BalanceBeforeUnits < transaction.AmountUnits || transaction.BalanceBeforeUnits-transaction.AmountUnits != transaction.BalanceAfterUnits {
		return nil, ErrHoldIntegrity
	}
	if err := validateHoldSummary(transaction.Remark); err != nil {
		return nil, ErrHoldIntegrity
	}
	return &transaction, nil
}

func validateReleasedHoldFact(tx *gorm.DB, hold *Hold, wallet *Wallet, userID int64) error {
	if hold == nil || wallet == nil || hold.UserID != userID || hold.WalletID != wallet.ID {
		return ErrHoldOwnerMismatch
	}
	transactions, err := terminalAIGenerateTransactions(tx, hold)
	if err != nil {
		return err
	}
	if hold.HeldUnits != 0 || hold.CapturedUnits != 0 || len(transactions) != 0 {
		return ErrHoldIntegrity
	}
	return nil
}
