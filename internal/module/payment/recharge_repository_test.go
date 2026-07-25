package payment

import (
	"context"
	"errors"
	"math"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"
	sharedmoney "admin_back_go/internal/shared/money"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGormRepositoryFinalizePaidOrderCommitsOrderRechargeAndWalletAtomically(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	units := int64(500 * 1_000_000)

	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaying, nil)
	expectLockedRecharge(mock, now, rechargeStatusPaying, nil, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_units", "total_recharge_units", "total_consume_units", "held_units", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1000*1_000_000), int64(1000*1_000_000), int64(0), int64(0), enum.CommonNo, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(walletSourceRecharge, int64(10), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnResult(sqlmock.NewResult(99, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET `balance_units`=?,`total_recharge_units`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_recharges` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if err != nil {
		t.Fatalf("FinalizePaidOrder error=%v", err)
	}
	if fact == nil || fact.Order == nil || fact.Recharge == nil || fact.Wallet == nil {
		t.Fatalf("expected committed order/recharge/wallet fact, got %#v", fact)
	}
	if !fact.OrderPaid || fact.OrderAlreadyPaid || !fact.RechargeCredited || fact.RechargeAlreadyCredited || fact.RawOrder {
		t.Fatalf("unexpected first-finalization flags: %#v", fact)
	}
	if fact.Wallet.BalanceUnits != 1500*1_000_000 || fact.Wallet.TotalRechargeUnits != 1500*1_000_000 || fact.Recharge.Status != rechargeStatusCredited || fact.Recharge.CreditedAt == nil {
		t.Fatalf("unexpected committed facts: %#v", fact)
	}
	if units != 500_000_000 {
		t.Fatalf("test conversion invariant changed: %d", units)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderCreditedReplayLoadsVerifiedWalletFact(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	units := int64(500 * 1000000)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaid, &paidAt)
	expectLockedRecharge(mock, now, rechargeStatusCredited, &paidAt, &now)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_units", "total_recharge_units", "total_consume_units", "held_units", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), units, units, int64(0), int64(0), enum.CommonNo, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(walletSourceRecharge, int64(10), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_units", "balance_before_units", "balance_after_units", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(9), "WLT20260726120000000001", int64(1), int64(7), walletDirectionIn, units, int64(0), units, walletSourceRecharge, int64(10), "支付宝充值", enum.CommonNo, now, now))
	mock.ExpectCommit()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if err != nil || fact == nil || fact.Wallet == nil || fact.Wallet.ID != 1 || fact.Recharge == nil || fact.Recharge.Status != rechargeStatusCredited {
		t.Fatalf("replay must return verified wallet and recharge facts, fact=%#v err=%v", fact, err)
	}
	if fact.OrderPaid || !fact.OrderAlreadyPaid || fact.RechargeCredited || !fact.RechargeAlreadyCredited || fact.RawOrder {
		t.Fatalf("unexpected credited replay flags: %#v", fact)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryListUncreditedPaidRechargesFindsPaidOrdersWithoutWalletCredit(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectQuery(`SELECT r\.\*, po\.order_no AS payment_order_no.*FROM payment_recharges AS r.*JOIN payment_orders AS po.*po\.status = \?.*r\.status IN.*r\.credited_at IS NULL.*r\.is_del = \?.*ORDER BY r\.id asc.*LIMIT \?`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recharge_no", "user_id", "package_code", "package_name", "amount_cents", "payment_order_id",
			"status", "paid_at", "credited_at", "failure_reason", "is_del", "created_at", "updated_at",
			"payment_order_no", "pay_url", "order_status", "alipay_trade_no", "order_paid_at",
		}).AddRow(
			int64(10), "RCG20260530115900000001", int64(7), "recharge_5", "¥5", int64(500), int64(20),
			rechargeStatusPaid, &paidAt, nil, "", enum.CommonNo, now, now,
			"PAY20260530115900000001", "", orderStatusPaid, "202605302200", &paidAt,
		))

	rows, err := repo.ListUncreditedPaidRecharges(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListUncreditedPaidRecharges error=%v", err)
	}
	if len(rows) != 1 || rows[0].ID != 10 || rows[0].PaymentOrderNo == "" || rows[0].OrderPaidAt == nil {
		t.Fatalf("unexpected rows=%#v", rows)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRejectsCreditedStatusWithoutCreditedAt(t *testing.T) {
	participant := &fakePaymentParticipant{
		wallet:      &walletmodule.Wallet{ID: 1, UserID: 7, IsDel: enum.CommonNo},
		transaction: &walletmodule.Transaction{ID: 9, WalletID: 1, UserID: 7, Direction: walletmodule.DirectionIn, AmountUnits: 500_000_000, SourceType: walletmodule.SourceRecharge, SourceID: 10, IsDel: enum.CommonNo},
	}
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaid, &paidAt)
	expectLockedRecharge(mock, now, rechargeStatusCredited, &paidAt, nil)
	mock.ExpectRollback()

	_, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("inconsistent credited fact must fail closed, got err=%v", err)
	}
	if participant.creditCalls != 0 {
		t.Fatalf("inconsistent credited fact must fail before wallet credit, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRejectsNilWalletParticipantFact(t *testing.T) {
	participant := &fakePaymentParticipant{}
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaying, nil)
	expectLockedRecharge(mock, now, rechargeStatusPaying, nil, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	_, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("nil wallet participant fact must fail closed, got err=%v", err)
	}
	if participant.creditCalls != 1 {
		t.Fatalf("expected one participant call, got %d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRollsBackOrderCASMiss(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaying, nil)
	expectLockedRecharge(mock, now, rechargeStatusPaying, nil, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("order CAS miss must fail closed, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 0 {
		t.Fatalf("order CAS miss must fail before wallet participant, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRollsBackRechargeCASMiss(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaid, &paidAt)
	expectLockedRecharge(mock, now, rechargeStatusPaid, &paidAt, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_recharges` SET")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("recharge CAS miss must roll back wallet credit, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 1 || participant.input.AmountUnits != 500_000_000 {
		t.Fatalf("expected one checked cents-to-units participant call, calls=%d input=%#v", participant.creditCalls, participant.input)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRollsBackParticipantError(t *testing.T) {
	participantErr := errors.New("wallet participant failed")
	participant := validFakePaymentParticipant()
	participant.err = participantErr
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaying, nil)
	expectLockedRecharge(mock, now, rechargeStatusPaying, nil, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, participantErr) {
		t.Fatalf("participant error must roll back outer transaction, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 1 {
		t.Fatalf("expected one participant call, got %d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRejectsForgedParticipantOwnerOnReplay(t *testing.T) {
	participant := validFakePaymentParticipant()
	participant.wallet.UserID = 8
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaid, &paidAt)
	expectLockedRecharge(mock, now, rechargeStatusCredited, &paidAt, &now)
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("forged participant owner must fail closed, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 1 {
		t.Fatalf("expected replay participant verification, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRejectsInvalidRechargeFacts(t *testing.T) {
	tests := []struct {
		name           string
		userID         int64
		amountCents    int64
		paymentOrderID int64
		isDel          int
	}{
		{name: "owner", userID: 0, amountCents: 500, paymentOrderID: 20, isDel: enum.CommonNo},
		{name: "amount", userID: 7, amountCents: 501, paymentOrderID: 20, isDel: enum.CommonNo},
		{name: "association", userID: 7, amountCents: 500, paymentOrderID: 21, isDel: enum.CommonNo},
		{name: "deleted", userID: 7, amountCents: 500, paymentOrderID: 20, isDel: enum.CommonYes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			participant := validFakePaymentParticipant()
			repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
			defer closeDB()

			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			paidAt := now.Add(-time.Minute)
			mock.ExpectBegin()
			expectLockedPaymentOrder(mock, now, orderStatusPaid, &paidAt)
			expectLockedRechargeFact(mock, now, 10, tt.userID, tt.amountCents, tt.paymentOrderID, rechargeStatusPaid, &paidAt, nil, tt.isDel)
			mock.ExpectRollback()

			fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
			if fact != nil || !errors.Is(err, ErrPaymentStateChanged) {
				t.Fatalf("invalid recharge fact must fail closed, fact=%#v err=%v", fact, err)
			}
			if participant.creditCalls != 0 {
				t.Fatalf("invalid recharge fact must fail before participant, calls=%d", participant.creditCalls)
			}
			assertPaymentMockExpectations(t, mock)
		})
	}
}

func TestGormRepositoryFinalizePaidOrderRejectsPaidRechargeWithCreditedAt(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaid, &paidAt)
	expectLockedRecharge(mock, now, rechargeStatusPaid, &paidAt, &now)
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("paid recharge with credited_at must fail closed, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 0 {
		t.Fatalf("inconsistent lifecycle must fail before participant, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRawOrderUsesSameTransaction(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaying, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `payment_recharges` WHERE payment_order_id = ? ORDER BY `payment_recharges`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(20), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if err != nil || fact == nil || fact.Order == nil || !fact.RawOrder || !fact.OrderPaid || fact.OrderAlreadyPaid || fact.Recharge != nil || fact.Wallet != nil {
		t.Fatalf("unexpected raw-order fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 0 {
		t.Fatalf("raw order must not call wallet participant, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRejectsInvalidOrderLifecycle(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectBegin()
	expectLockedPaymentOrder(mock, now, orderStatusPaid, nil)
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("paid order without paid_at must fail closed, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 0 {
		t.Fatalf("invalid order must fail before recharge/wallet work, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderReturnsTypedNotFound(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `payment_orders` WHERE id = ? ORDER BY `payment_orders`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(20), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", now.Add(-time.Minute), now)
	if fact != nil || !errors.Is(err, ErrPaymentOrderNotFound) {
		t.Fatalf("missing order must return typed not-found, fact=%#v err=%v", fact, err)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryFinalizePaidOrderRollsBackCentsConversionOverflow(t *testing.T) {
	participant := validFakePaymentParticipant()
	repo, mock, closeDB := newPaymentMockRepositoryWithParticipant(t, participant)
	defer closeDB()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	overflowCents := int64(math.MaxInt64/sharedmoney.UnitsPerCent + 1)
	mock.ExpectBegin()
	expectLockedPaymentOrderFact(mock, now, orderStatusPaid, overflowCents, &paidAt, enum.CommonNo)
	expectLockedRechargeFact(mock, now, 10, 7, overflowCents, 20, rechargeStatusPaid, &paidAt, nil, enum.CommonNo)
	mock.ExpectRollback()

	fact, err := repo.FinalizePaidOrder(context.Background(), 20, "202607262200", paidAt, now)
	if fact != nil || !errors.Is(err, sharedmoney.ErrInvalidAmount) {
		t.Fatalf("overflow conversion must fail closed, fact=%#v err=%v", fact, err)
	}
	if participant.creditCalls != 0 {
		t.Fatalf("overflow must fail before participant, calls=%d", participant.creditCalls)
	}
	assertPaymentMockExpectations(t, mock)
}

type fakePaymentParticipant struct {
	wallet      *walletmodule.Wallet
	transaction *walletmodule.Transaction
	err         error
	creditCalls int
	input       walletmodule.CreditRechargeInput
}

func validFakePaymentParticipant() *fakePaymentParticipant {
	const units = int64(500_000_000)
	return &fakePaymentParticipant{
		wallet: &walletmodule.Wallet{
			ID: 1, UserID: 7, BalanceUnits: units, TotalRechargeUnits: units, IsDel: enum.CommonNo,
		},
		transaction: &walletmodule.Transaction{
			ID: 9, TransactionNo: "WLT20260726120000000001", WalletID: 1, UserID: 7,
			Direction: walletmodule.DirectionIn, AmountUnits: units, BalanceBeforeUnits: 0, BalanceAfterUnits: units,
			SourceType: walletmodule.SourceRecharge, SourceID: 10, IsDel: enum.CommonNo,
		},
	}
}

func (p *fakePaymentParticipant) CreditRechargeInTx(_ context.Context, _ *gorm.DB, input walletmodule.CreditRechargeInput) (*walletmodule.Wallet, *walletmodule.Transaction, error) {
	p.creditCalls++
	p.input = input
	return p.wallet, p.transaction, p.err
}

func (p *fakePaymentParticipant) GetOrCreateWallet(context.Context, int64) (*walletmodule.Wallet, error) {
	return p.wallet, p.err
}

func (p *fakePaymentParticipant) GetWallet(context.Context, int64) (*walletmodule.Wallet, error) {
	return p.wallet, p.err
}

func expectLockedPaymentOrder(mock sqlmock.Sqlmock, now time.Time, status string, paidAt *time.Time) {
	expectLockedPaymentOrderFact(mock, now, status, 500, paidAt, enum.CommonNo)
}

func expectLockedPaymentOrderFact(mock sqlmock.Sqlmock, now time.Time, status string, amountCents int64, paidAt *time.Time, isDel int) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `payment_orders` WHERE id = ? ORDER BY `payment_orders`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(20), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_no", "config_id", "config_code", "provider", "pay_method", "subject", "amount_cents", "status", "pay_url", "return_url", "alipay_trade_no", "expired_at", "paid_at", "closed_at", "failure_reason", "is_del", "created_at", "updated_at"}).
			AddRow(int64(20), "PAY20260726115900000001", int64(1), "alipay_default", providerAlipay, enum.PaymentMethodWeb, "余额充值", amountCents, status, "", "", "202607262200", now.Add(time.Hour), paidAt, nil, "", isDel, now, now))
}

func expectLockedRecharge(mock sqlmock.Sqlmock, now time.Time, status string, paidAt, creditedAt *time.Time) {
	expectLockedRechargeFact(mock, now, 10, 7, 500, 20, status, paidAt, creditedAt, enum.CommonNo)
}

func expectLockedRechargeFact(mock sqlmock.Sqlmock, now time.Time, id, userID, amountCents, paymentOrderID int64, status string, paidAt, creditedAt *time.Time, isDel int) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `payment_recharges` WHERE payment_order_id = ? ORDER BY `payment_recharges`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(20), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recharge_no", "user_id", "package_code", "package_name", "amount_cents", "payment_order_id", "status", "paid_at", "credited_at", "failure_reason", "is_del", "created_at", "updated_at"}).
			AddRow(id, "RCG20260726115900000001", userID, "recharge_5", "¥5", amountCents, paymentOrderID, status, paidAt, creditedAt, "", isDel, now, now))
}

func newPaymentMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	return newPaymentMockRepositoryWithParticipant(t, nil)
}

func newPaymentMockRepositoryWithParticipant(t *testing.T, participant walletmodule.PaymentParticipant) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm mock db: %v", err)
	}
	client := &database.Client{Gorm: db, SQL: sqlDB}
	if participant == nil {
		participant = walletmodule.NewGormRepository(client)
	}
	return NewGormRepository(client, participant), mock, func() { _ = sqlDB.Close() }
}

func assertPaymentMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
