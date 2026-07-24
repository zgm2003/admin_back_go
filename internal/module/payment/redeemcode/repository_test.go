package redeemcode

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var _ Repository = (*GormRepository)(nil)

func TestRepositoryCreateBatchAndCodesUsesOneTransaction(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 123456000, time.UTC)
	repository, participant, mock, closeDB := newMockRedeemRepository(t, clock.Func(func() time.Time { return now }))
	defer closeDB()
	_ = participant
	record := validCreateRecord(now.Add(time.Hour))

	mock.ExpectBegin()
	expectBatchByRequest(mock, 7, "request-1").WillReturnRows(batchRows())
	mock.ExpectExec("INSERT INTO `redeem_code_batches`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec("INSERT INTO `redeem_codes`").WillReturnResult(sqlmock.NewResult(20, 2))
	mock.ExpectCommit()

	created, replayed, err := repository.CreateOrReplayBatch(context.Background(), record)
	if err != nil || replayed || created == nil || created.Batch.ID != 10 || len(created.Codes) != 2 || created.Codes[0].BatchID != 10 {
		t.Fatalf("CreateOrReplayBatch=(%+v,%v,%v)", created, replayed, err)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryCreateRejectsExpiryAtTransactionCreatedAt(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository, _, mock, closeDB := newMockRedeemRepository(t, clock.Func(func() time.Time { return now }))
	defer closeDB()
	record := validCreateRecord(now)

	mock.ExpectBegin()
	expectBatchByRequest(mock, 7, "request-1").WillReturnRows(batchRows())
	mock.ExpectRollback()
	created, replayed, err := repository.CreateOrReplayBatch(context.Background(), record)
	if !errors.Is(err, ErrExpiryNotFuture) || created != nil || replayed {
		t.Fatalf("CreateOrReplayBatch=(%+v,%v,%v)", created, replayed, err)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryCreatorRequestRaceReplaysCommittedBatch(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository, _, mock, closeDB := newMockRedeemRepository(t, clock.Func(func() time.Time { return now }))
	defer closeDB()
	record := validCreateRecord(now.Add(time.Hour))

	mock.ExpectBegin()
	expectBatchByRequest(mock, 7, "request-1").WillReturnRows(batchRows())
	mock.ExpectExec("INSERT INTO `redeem_code_batches`").WillReturnError(&mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry for key 'uk_redeem_code_batches_creator_request'"})
	mock.ExpectRollback()
	expectBatchByRequest(mock, 7, "request-1").WillReturnRows(batchRows().AddRow(
		int64(10), record.Batch.BatchNo, record.Batch.RequestID, record.Batch.RequestFingerprintVersion,
		record.Batch.RequestFingerprint, record.Batch.AmountCents, record.Batch.Quantity, record.Batch.ExpiresAt,
		record.Batch.Note, record.Batch.CreatedBy, now, now,
	))
	expectCodesByBatch(mock, 10, record.Batch.Quantity+1).WillReturnRows(codeRows().
		AddRow(int64(20), int64(10), record.Codes[0].Code, StateUnused, nil, nil, now, now).
		AddRow(int64(21), int64(10), record.Codes[1].Code, StateUnused, nil, nil, now, now))

	batch, replayed, err := repository.CreateOrReplayBatch(context.Background(), record)
	if err != nil || !replayed || batch == nil || len(batch.Codes) != 2 {
		t.Fatalf("CreateOrReplayBatch=(%+v,%v,%v)", batch, replayed, err)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryCodeCollisionReturnsSanitizedSentinel(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository, _, mock, closeDB := newMockRedeemRepository(t, clock.Func(func() time.Time { return now }))
	defer closeDB()
	record := validCreateRecord(now.Add(time.Hour))
	rawCode := record.Codes[0].Code

	mock.ExpectBegin()
	expectBatchByRequest(mock, 7, "request-1").WillReturnRows(batchRows())
	mock.ExpectExec("INSERT INTO `redeem_code_batches`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec("INSERT INTO `redeem_codes`").WillReturnError(&mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry '" + rawCode + "' for key 'uk_redeem_codes_code'"})
	mock.ExpectRollback()
	_, _, err := repository.CreateOrReplayBatch(context.Background(), record)
	if !errors.Is(err, ErrCodeCollision) || strings.Contains(err.Error(), rawCode) {
		t.Fatalf("error=%v", err)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryFindBatchRequiresExactSortedQuantity(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository, _, mock, closeDB := newMockRedeemRepository(t, nil)
	defer closeDB()
	record := validCreateRecord(now.Add(time.Hour))
	expectBatchByRequest(mock, 7, "request-1").WillReturnRows(batchRows().AddRow(
		int64(10), record.Batch.BatchNo, record.Batch.RequestID, record.Batch.RequestFingerprintVersion,
		record.Batch.RequestFingerprint, record.Batch.AmountCents, 2, record.Batch.ExpiresAt, record.Batch.Note, int64(7), now, now,
	))
	expectCodesByBatch(mock, 10, 3).WillReturnRows(codeRows().
		AddRow(int64(20), int64(10), record.Codes[0].Code, StateUnused, nil, nil, now, now))
	batch, err := repository.FindBatchByRequest(context.Background(), 7, "request-1")
	if !errors.Is(err, ErrIntegrityViolation) || batch != nil {
		t.Fatalf("FindBatchByRequest=(%+v,%v)", batch, err)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryVoidLocksSortedSetAndIsAllOrNothing(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository, _, mock, closeDB := newMockRedeemRepository(t, nil)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `redeem_codes` WHERE id IN .* ORDER BY id ASC LIMIT .* FOR UPDATE").
		WithArgs(int64(1), int64(2), int64(3), 4).
		WillReturnRows(codeRows().
			AddRow(int64(1), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUnused, nil, nil, now, now).
			AddRow(int64(2), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMP", StateVoided, nil, nil, now, now).
			AddRow(int64(3), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMQ", StateUnused, nil, nil, now, now))
	mock.ExpectExec("UPDATE `redeem_codes` SET .* WHERE id IN .* AND state =").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	count, err := repository.VoidCodes(context.Background(), []int64{3, 1, 3, 2}, now)
	if err != nil || count != 2 {
		t.Fatalf("VoidCodes=(%d,%v)", count, err)
	}
	assertSQLMock(t, mock)

	repository, _, mock, closeDB = newMockRedeemRepository(t, nil)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `redeem_codes` WHERE id IN .* ORDER BY id ASC LIMIT .* FOR UPDATE").
		WithArgs(int64(1), int64(2), 3).
		WillReturnRows(codeRows().AddRow(int64(1), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUsed, int64(7), now, now, now))
	mock.ExpectRollback()
	count, err = repository.VoidCodes(context.Background(), []int64{1, 2}, now)
	if !errors.Is(err, ErrVoidConflict) || count != 0 {
		t.Fatalf("VoidCodes conflict=(%d,%v)", count, err)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryRedeemUsesDecisionTimeAfterCodeLock(t *testing.T) {
	enteredAt := time.Date(2026, 7, 24, 11, 59, 59, 0, time.UTC)
	expires := enteredAt.Add(time.Second)
	decisionTime := expires.Add(time.Microsecond)
	fakeClock := &countingClock{times: []time.Time{decisionTime}}
	repository, participant, mock, closeDB := newMockRedeemRepository(t, fakeClock)
	defer closeDB()

	mock.ExpectBegin()
	expectCodeForUpdate(mock, "ZHR-2345-6789-ABCD-EFGH-JKMN").WillReturnRows(codeRows().
		AddRow(int64(20), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUnused, nil, nil, enteredAt, enteredAt))
	expectBatchByID(mock, 10).WillReturnRows(batchRows().AddRow(int64(10), "RCB1", "request-1", RequestFingerprintVersion,
		strings.Repeat("a", 64), int64(100), 1, expires, "", int64(7), enteredAt, enteredAt))
	mock.ExpectRollback()
	fact, err := repository.Redeem(context.Background(), 8, "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrExpired) || fact != nil || participant.creditCalls != 0 || participant.findCalls != 0 {
		t.Fatalf("Redeem=(%+v,%v) participant=%+v", fact, err, participant)
	}
	if fakeClock.calls != 1 {
		t.Fatalf("decision clock calls=%d", fakeClock.calls)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryRedeemOwnUsedCodeReplaysBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	expires := now.Add(-time.Hour)
	usedAt := now.Add(-24 * time.Hour)
	fakeClock := &countingClock{times: []time.Time{now}}
	repository, participant, mock, closeDB := newMockRedeemRepository(t, fakeClock)
	defer closeDB()
	participant.findWallet = &wallet.Wallet{ID: 4, UserID: 8, BalanceCents: 999, TotalRechargeCents: 100, IsDel: enum.CommonNo}
	participant.findTransaction = &wallet.Transaction{ID: 9, TransactionNo: "WLT1", WalletID: 4, UserID: 8, Direction: wallet.DirectionIn,
		AmountCents: 100, BalanceBeforeCents: 0, BalanceAfterCents: 100, SourceType: wallet.SourceRedeemCode, SourceID: 20,
		Remark: "RCB1", IsDel: enum.CommonNo, CreatedAt: usedAt}
	mock.ExpectBegin()
	expectCodeForUpdate(mock, "ZHR-2345-6789-ABCD-EFGH-JKMN").WillReturnRows(codeRows().
		AddRow(int64(20), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUsed, int64(8), usedAt, usedAt, usedAt))
	expectBatchByID(mock, 10).WillReturnRows(batchRows().AddRow(int64(10), "RCB1", "request-1", RequestFingerprintVersion,
		strings.Repeat("a", 64), int64(100), 1, expires, "", int64(7), usedAt, usedAt))
	mock.ExpectCommit()
	fact, err := repository.Redeem(context.Background(), 8, "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if err != nil || fact == nil || !fact.Replayed || fact.Wallet.BalanceCents != 999 || participant.findCalls != 1 || participant.creditCalls != 0 {
		t.Fatalf("Redeem=(%+v,%v) participant=%+v", fact, err, participant)
	}
	if fakeClock.calls != 0 {
		t.Fatalf("replay must not evaluate current expiry, clock calls=%d", fakeClock.calls)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryRedeemCreditsWalletThenMarksCodeUsed(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	repository, participant, mock, closeDB := newMockRedeemRepository(t, clock.Func(func() time.Time { return now }))
	defer closeDB()
	participant.creditWallet = &wallet.Wallet{ID: 4, UserID: 8, BalanceCents: 100, TotalRechargeCents: 100, IsDel: enum.CommonNo}
	participant.creditTransaction = &wallet.Transaction{ID: 9, TransactionNo: "WLT1", WalletID: 4, UserID: 8, Direction: wallet.DirectionIn,
		AmountCents: 100, BalanceBeforeCents: 0, BalanceAfterCents: 100, SourceType: wallet.SourceRedeemCode, SourceID: 20,
		Remark: "RCB1", IsDel: enum.CommonNo, CreatedAt: now}
	mock.ExpectBegin()
	expectCodeForUpdate(mock, "ZHR-2345-6789-ABCD-EFGH-JKMN").WillReturnRows(codeRows().
		AddRow(int64(20), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUnused, nil, nil, now, now))
	expectBatchByID(mock, 10).WillReturnRows(batchRows().AddRow(int64(10), "RCB1", "request-1", RequestFingerprintVersion,
		strings.Repeat("a", 64), int64(100), 1, expires, "", int64(7), now, now))
	mock.ExpectExec("UPDATE `redeem_codes` SET .* WHERE id = .* AND state =").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	fact, err := repository.Redeem(context.Background(), 8, "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if err != nil || fact == nil || fact.Replayed || participant.creditCalls != 1 || participant.lastCredit.CodeID != 20 || participant.lastCredit.BatchNo != "RCB1" {
		t.Fatalf("Redeem=(%+v,%v) participant=%+v", fact, err, participant)
	}
	assertSQLMock(t, mock)
}

func TestRepositoryRedeemMapsWalletSourceAndOverflowToIntegrity(t *testing.T) {
	tests := []struct {
		walletErr error
		want      error
	}{
		{wallet.ErrRedeemCodeSourceExists, ErrSourceConflict},
		{wallet.ErrRedeemCodeBalanceOverflow, ErrOverflow},
		{wallet.ErrRedeemCodeTotalRechargeOverflow, ErrOverflow},
		{wallet.ErrRedeemCodeWalletIntegrity, ErrIntegrityViolation},
	}
	for _, test := range tests {
		now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
		repository, participant, mock, closeDB := newMockRedeemRepository(t, clock.Func(func() time.Time { return now }))
		participant.creditErr = test.walletErr
		mock.ExpectBegin()
		expectCodeForUpdate(mock, "ZHR-2345-6789-ABCD-EFGH-JKMN").WillReturnRows(codeRows().
			AddRow(int64(20), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUnused, nil, nil, now, now))
		expectBatchByID(mock, 10).WillReturnRows(batchRows().AddRow(int64(10), "RCB1", "request-1", RequestFingerprintVersion,
			strings.Repeat("a", 64), int64(100), 1, now.Add(time.Hour), "", int64(7), now, now))
		mock.ExpectRollback()
		fact, err := repository.Redeem(context.Background(), 8, "ZHR-2345-6789-ABCD-EFGH-JKMN")
		if !errors.Is(err, test.want) || !errors.Is(err, ErrIntegrityViolation) || fact != nil {
			t.Fatalf("walletErr=%v Redeem=(%+v,%v)", test.walletErr, fact, err)
		}
		assertSQLMock(t, mock)
		closeDB()
	}
}

func TestRepositoryListLookupAndExportUseBoundedJoinReadModel(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository, _, mock, closeDB := newMockRedeemRepository(t, nil)
	defer closeDB()
	readRows := codeViewRows().AddRow(int64(20), int64(10), "ZHR-2345-6789-ABCD-EFGH-JKMN", StateUnused, nil, nil, now,
		"RCB1", int64(100), nil, "", int64(7), "creator", "", "", "")
	mock.ExpectQuery("SELECT count.* FROM redeem_codes AS rc .*JOIN redeem_code_batches").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM redeem_codes AS rc .*JOIN redeem_code_batches.*ORDER BY rc.id DESC LIMIT").WillReturnRows(readRows)
	rows, total, err := repository.ListCodes(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20}, now)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("ListCodes=(%+v,%d,%v)", rows, total, err)
	}

	mock.ExpectQuery("SELECT .* FROM redeem_codes AS rc .*JOIN redeem_code_batches.*rc.code = .*ORDER BY rc.id ASC LIMIT").
		WithArgs(enum.CommonNo, enum.CommonNo, wallet.SourceRedeemCode, enum.CommonNo, "ZHR-2345-6789-ABCD-EFGH-JKMN", 2).
		WillReturnRows(codeViewRows())
	lookup, err := repository.LookupCode(context.Background(), "ZHR-2345-6789-ABCD-EFGH-JKMN", now)
	if err != nil || lookup != nil {
		t.Fatalf("LookupCode=(%+v,%v)", lookup, err)
	}

	mock.ExpectQuery("SELECT .* FROM redeem_codes AS rc .*JOIN redeem_code_batches.*ORDER BY rc.id DESC LIMIT").
		WithArgs(enum.CommonNo, enum.CommonNo, wallet.SourceRedeemCode, enum.CommonNo, MaxExportRows+1).
		WillReturnRows(codeViewRows())
	exported, err := repository.ExportCodes(context.Background(), ListQuery{}, now, MaxExportRows+1)
	if err != nil || len(exported) != 0 {
		t.Fatalf("ExportCodes=(%+v,%v)", exported, err)
	}
	assertSQLMock(t, mock)
}

type fakeWalletParticipant struct {
	findCalls         int
	findWallet        *wallet.Wallet
	findTransaction   *wallet.Transaction
	findErr           error
	creditCalls       int
	creditWallet      *wallet.Wallet
	creditTransaction *wallet.Transaction
	creditErr         error
	lastCredit        wallet.RedeemCodeCreditInput
}

func (participant *fakeWalletParticipant) FindRedeemCodeCreditInTx(context.Context, *gorm.DB, int64, bool) (*wallet.Wallet, *wallet.Transaction, error) {
	participant.findCalls++
	return participant.findWallet, participant.findTransaction, participant.findErr
}

func (participant *fakeWalletParticipant) CreditRedeemCodeInTx(_ context.Context, _ *gorm.DB, input wallet.RedeemCodeCreditInput, _ time.Time) (*wallet.Wallet, *wallet.Transaction, error) {
	participant.creditCalls++
	participant.lastCredit = input
	return participant.creditWallet, participant.creditTransaction, participant.creditErr
}

func newMockRedeemRepository(t *testing.T, fakeClock clock.Clock) (*GormRepository, *fakeWalletParticipant, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	participant := &fakeWalletParticipant{}
	client := &database.Client{Gorm: db, SQL: sqlDB}
	repository := NewGormRepository(client, participant, fakeClock)
	return repository, participant, mock, func() { _ = sqlDB.Close() }
}

func validCreateRecord(expires time.Time) CreateBatchRecord {
	return CreateBatchRecord{Batch: Batch{
		BatchNo: "RCB202607241200000001", RequestID: "request-1", RequestFingerprintVersion: RequestFingerprintVersion,
		RequestFingerprint: strings.Repeat("a", 64), AmountCents: 100, Quantity: 2, ExpiresAt: &expires,
		Note: "campaign", CreatedBy: 7,
	}, Codes: []Code{
		{Code: "ZHR-2345-6789-ABCD-EFGH-JKMN", State: StateUnused},
		{Code: "ZHR-2345-6789-ABCD-EFGH-JKMP", State: StateUnused},
	}}
}

func batchRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "batch_no", "request_id", "request_fingerprint_version", "request_fingerprint", "amount_cents", "quantity", "expires_at", "note", "created_by", "created_at", "updated_at"})
}

func codeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "batch_id", "code", "state", "used_by", "used_at", "created_at", "updated_at"})
}

func codeViewRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "batch_id", "code", "state", "used_by", "used_at", "created_at", "batch_no", "amount_cents", "expires_at", "note", "created_by", "creator_username", "used_username", "used_account", "wallet_transaction_no"})
}

func expectBatchByRequest(mock sqlmock.Sqlmock, createdBy int64, requestID string) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `redeem_code_batches` WHERE created_by = ? AND request_id = ? ORDER BY id ASC LIMIT ?")).
		WithArgs(createdBy, requestID, 2)
}

func expectCodesByBatch(mock sqlmock.Sqlmock, batchID int64, limit int) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `redeem_codes` WHERE batch_id = ? ORDER BY id ASC LIMIT ?")).WithArgs(batchID, limit)
}

func expectCodeForUpdate(mock sqlmock.Sqlmock, code string) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `redeem_codes` WHERE code = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).WithArgs(code, 2)
}

func expectBatchByID(mock sqlmock.Sqlmock, batchID int64) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `redeem_code_batches` WHERE id = ? ORDER BY `redeem_code_batches`.`id` LIMIT ?")).WithArgs(batchID, 1)
}

func assertSQLMock(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
