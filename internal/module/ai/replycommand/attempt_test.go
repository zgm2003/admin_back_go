package replycommand

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPrepareLegacyAttemptUsesDedicatedInput(t *testing.T) {
	method, ok := reflect.TypeOf((*GormRepository)(nil)).MethodByName("PrepareLegacyAttempt")
	if !ok {
		t.Fatal("PrepareLegacyAttempt is missing")
	}
	if got := method.Type.In(2).Name(); got != "LegacyPrepareAttemptInput" {
		t.Fatalf("legacy prepare input type=%q, want LegacyPrepareAttemptInput", got)
	}
	inputType := method.Type.In(2)
	for _, paidField := range []string{"AttemptNo", "IdempotencyKey", "PreparedRequestJSON", "PreparedRequestSHA256", "QuoteJSON"} {
		if _, exists := inputType.FieldByName(paidField); exists {
			t.Fatalf("legacy prepare input exposes paid field %s", paidField)
		}
	}
}

func TestLegacyAttemptUsesExactDurableV1Markers(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt_no\), 0\) FROM ` + "`ai_provider_attempts`" + ` WHERE run_id = \?`).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(attempt_no), 0)"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `ai_provider_attempts`").WillReturnResult(sqlmock.NewResult(91, 1))
	mock.ExpectCommit()

	attempt, ok, err := repository.PrepareLegacyAttempt(context.Background(), LegacyPrepareAttemptInput{RunID: 100, CommandID: 41, Owner: "worker-a", Token: 7, Now: now})
	if err != nil || !ok || attempt == nil {
		t.Fatalf("attempt=%+v ok=%v err=%v", attempt, ok, err)
	}
	if want := `{"version":"legacy_unavailable_v1","replayable":false}`; attempt.PreparedRequestJSON != want {
		t.Fatalf("prepared request marker=%s, want %s", attempt.PreparedRequestJSON, want)
	}
	if want := `{"version":"legacy_unpriced_v1","billable":false}`; attempt.QuoteJSON != want {
		t.Fatalf("quote marker=%s, want %s", attempt.QuoteJSON, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparePaidAttemptInTransactionRejectsNilAndRootHandles(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	input := validPaidPrepareInput()

	for _, tc := range []struct {
		name string
		tx   *gorm.DB
	}{
		{name: "nil", tx: nil},
		{name: "root", tx: root},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempt, ok, err := repository.PreparePaidAttemptInTransaction(context.Background(), tc.tx, input)
			if !errors.Is(err, ErrAttemptTransactionRequired) || ok || attempt != nil {
				t.Fatalf("attempt=%+v ok=%v err=%v", attempt, ok, err)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFinishAttemptRejectsInvalidTerminalEvidenceBeforeSQL(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	base := validFinishAttemptInput(t)

	for _, tc := range []struct {
		name   string
		mutate func(*FinishAttemptInput)
	}{
		{name: "succeeded usage has no raw proof", mutate: func(input *FinishAttemptInput) {
			input.UsageJSON = `{"status":"complete","items":[{"category":"output","unit":"token","quantity":1}]}`
		}},
		{name: "succeeded dispatch is unknown", mutate: func(input *FinishAttemptInput) { input.DispatchState = "unknown" }},
		{name: "outcome unknown contains complete usage", mutate: func(input *FinishAttemptInput) { input.State = AttemptOutcomeUnknown; input.DispatchState = "unknown" }},
		{name: "not dispatched failure has response proof", mutate: func(input *FinishAttemptInput) { input.State = AttemptFailed; input.DispatchState = "not_dispatched" }},
		{name: "complete canceled result has no provider id", mutate: func(input *FinishAttemptInput) { input.State = AttemptCanceled; input.ProviderRequestID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.mutate(&input)
			ok, err := repository.FinishAttempt(context.Background(), input)
			if !errors.Is(err, ErrCreateInputInvalid) || ok {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFinishAttemptAcceptsIdenticalTerminalReplayAndRejectsConflict(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	input := validFinishAttemptInput(t)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_provider_attempts` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if ok, err := repository.FinishAttempt(context.Background(), input); err != nil || !ok {
		t.Fatalf("first finish ok=%v err=%v", ok, err)
	}

	terminalRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "run_id", "state", "provider_request_id", "response_sha256", "error_code", "usage_json", "usage_status", "dispatch_state", "result_candidate_json", "finished_at"}).
			AddRow(input.AttemptID, input.RunID, input.State, input.ProviderRequestID, input.ResponseSHA256, input.ErrorCode, input.UsageJSON, input.UsageStatus, input.DispatchState, nil, input.Now)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_provider_attempts` SET").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_provider_attempts` WHERE id = ? AND run_id = ? ORDER BY `ai_provider_attempts`.`id` LIMIT ?")).
		WithArgs(input.AttemptID, input.RunID, 1).
		WillReturnRows(terminalRows())
	if ok, err := repository.FinishAttempt(context.Background(), input); err != nil || !ok {
		t.Fatalf("identical replay ok=%v err=%v", ok, err)
	}

	conflict := input
	conflict.ProviderRequestID = "provider-other"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_provider_attempts` SET").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_provider_attempts` WHERE id = ? AND run_id = ? ORDER BY `ai_provider_attempts`.`id` LIMIT ?")).
		WithArgs(input.AttemptID, input.RunID, 1).
		WillReturnRows(terminalRows())
	if ok, err := repository.FinishAttempt(context.Background(), conflict); !errors.Is(err, ErrAttemptTerminalConflict) || ok {
		t.Fatalf("conflicting replay ok=%v err=%v", ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func validFinishAttemptInput(t *testing.T) FinishAttemptInput {
	t.Helper()
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusComplete, []byte(`{"output_tokens":1}`), []infraai.UsageItem{{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	responseHash := sha256.Sum256([]byte("provider-response"))
	return FinishAttemptInput{
		RunID:             44,
		AttemptID:         71,
		State:             AttemptSucceeded,
		ProviderRequestID: "provider-request",
		ResponseSHA256:    hex.EncodeToString(responseHash[:]),
		DispatchState:     infraai.DispatchStateDispatched,
		UsageJSON:         string(usageJSON),
		UsageStatus:       "complete",
		Now:               time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC),
	}
}

func TestPreparePaidAttemptUsesSuppliedTransactionWithoutNestedBegin(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	tx := root.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_provider_attempts` WHERE attempt_no = ? AND run_id = ? ORDER BY `ai_provider_attempts`.`id` LIMIT ?")).
		WithArgs(uint(1), int64(44), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(attempt_no), 0) FROM `ai_provider_attempts` WHERE run_id = ?")).
		WithArgs(int64(44)).
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(attempt_no), 0)"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `ai_provider_attempts`").
		WillReturnResult(sqlmock.NewResult(71, 1))

	attempt, ok, err := repository.PreparePaidAttemptInTransaction(context.Background(), tx, validPaidPrepareInput())
	if err != nil || !ok || attempt == nil || attempt.ID != 71 || attempt.RunID != 44 || attempt.AttemptNo != 1 {
		t.Fatalf("attempt=%+v ok=%v err=%v", attempt, ok, err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAttemptTransactionGuardRejectsTypedNilAndAcceptsSQLTransaction(t *testing.T) {
	_, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()

	var nilTx *sql.Tx
	typedNil := &gorm.DB{Config: root.Config, Statement: &gorm.Statement{ConnPool: nilTx, Context: context.Background()}}
	typedNil.Statement.DB = typedNil
	if err := requireAttemptTransaction(typedNil); !errors.Is(err, ErrAttemptTransactionRequired) {
		t.Fatalf("typed-nil transaction error=%v", err)
	}

	mock.ExpectBegin()
	tx := root.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := requireAttemptTransaction(tx); err != nil {
		t.Fatalf("active transaction rejected: %v", err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func validPaidPrepareInput() PrepareAttemptInput {
	request := `{"model":"gpt-test"}`
	return PrepareAttemptInput{
		RunID:                 44,
		AttemptNo:             1,
		Owner:                 "worker",
		Token:                 1,
		IdempotencyKey:        providerAttemptKey(44, 1),
		PreparedRequestJSON:   request,
		PreparedRequestSHA256: sha256.Sum256([]byte(request)),
		QuoteJSON:             `{"pricing_version":"v1","target_hold_units":1}`,
	}
}

func newAttemptMockRepository(t *testing.T) (*GormRepository, *gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return newGormRepository(db), db, mock, func() { _ = sqlDB.Close() }
}
