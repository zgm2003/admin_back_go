package replycommand

import (
	"bytes"
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
	"admin_back_go/internal/module/ai/aigateway"

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

func TestFinishAttemptPersistsDrainedEvidenceAfterDurableCancellation(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	input := validFinishAttemptInput(t)
	input.CommandID = 41
	input.Owner = "worker-a"
	input.Token = 7
	input.State = AttemptCanceled
	input.ErrorCode = "ai.user_stopped"
	candidate := `{"version":"ai_chat_result_v1","answer":"drained"}`
	input.ResultCandidateJSON = &candidate

	mock.ExpectBegin()
	mock.ExpectExec(`^UPDATE ` + "`ai_provider_attempts`" + ` SET .*WHERE \(id = \? AND run_id = \? AND state = \?\) AND command_id = \? AND \(EXISTS \(SELECT 1 FROM ai_reply_commands c WHERE c.id = \? AND c.lease_owner = \? AND c.lease_token = \? AND c.state = \? AND c.lease_expires_at > \?\)\)$`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if ok, err := repository.FinishAttempt(context.Background(), input); err != nil || !ok {
		t.Fatalf("finish drained attempt ok=%v err=%v", ok, err)
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

func TestPrepareStartedAtIsPersistedOnlyWithHeldAttempt(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	prepareStartedAt := time.Date(2026, 7, 28, 10, 0, 3, 123456000, time.UTC)
	input := validPaidPrepareInput()
	input.PrepareStartedAt = prepareStartedAt

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
	mock.ExpectExec("INSERT INTO `ai_provider_attempts`").WillReturnResult(sqlmock.NewResult(71, 1))

	attempt, ok, err := repository.PreparePaidAttemptInTransaction(context.Background(), tx, input)
	if err != nil || !ok || attempt == nil || attempt.PrepareStartedAt == nil || !attempt.PrepareStartedAt.Equal(prepareStartedAt) {
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

func TestPreparePaidAttemptPersistsContextPlanEvidence(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	planHash := sha256.Sum256([]byte("ready plan"))
	input := validPaidPrepareInput()
	input.ContextPlan = &aigateway.ContextPlanEvidence{ID: 91, SHA256: planHash}

	mock.ExpectBegin()
	tx := root.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_provider_attempts` WHERE attempt_no = ? AND run_id = ? ORDER BY `ai_provider_attempts`.`id` LIMIT ?")).
		WithArgs(uint(1), int64(44), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_plans` WHERE id = ? ORDER BY `ai_context_plans`.`id` LIMIT ?")).
		WithArgs(uint64(91), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "state", "plan_sha256"}).AddRow(uint64(91), uint64(44), "ready", planHash[:]))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(attempt_no), 0) FROM `ai_provider_attempts` WHERE run_id = ?")).
		WithArgs(int64(44)).
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(attempt_no), 0)"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `ai_provider_attempts`").WillReturnResult(sqlmock.NewResult(71, 1))

	attempt, ok, err := repository.PreparePaidAttemptInTransaction(context.Background(), tx, input)
	if err != nil || !ok || attempt == nil || attempt.ContextPlanID == nil || *attempt.ContextPlanID != 91 || !bytes.Equal(attempt.ContextPlanSHA256, planHash[:]) {
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

func TestValidateContextPlanEvidenceInTransactionRejectsRecoveryConflicts(t *testing.T) {
	planHash := sha256.Sum256([]byte("ready plan"))
	otherHash := sha256.Sum256([]byte("other plan"))
	for _, test := range []struct {
		name  string
		runID uint64
		state string
		hash  []byte
	}{
		{name: "wrong run", runID: 45, state: "ready", hash: planHash[:]},
		{name: "failed plan", runID: 44, state: "failed", hash: planHash[:]},
		{name: "wrong hash", runID: 44, state: "ready", hash: otherHash[:]},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, root, mock, closeDB := newAttemptMockRepository(t)
			defer closeDB()
			mock.ExpectBegin()
			tx := root.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_plans` WHERE id = ? ORDER BY `ai_context_plans`.`id` LIMIT ?")).
				WithArgs(uint64(91), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "state", "plan_sha256"}).AddRow(uint64(91), test.runID, test.state, test.hash))

			err := ValidateContextPlanEvidenceInTransaction(context.Background(), tx, 44, &aigateway.ContextPlanEvidence{ID: 91, SHA256: planHash})
			if err == nil {
				t.Fatal("conflicting context plan recovery was accepted")
			}
			mock.ExpectRollback()
			if err := tx.Rollback().Error; err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparePaidAttemptRejectsContextPlanOverwrite(t *testing.T) {
	repository, root, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	input := validPaidPrepareInput()
	input.ContextPlan = &aigateway.ContextPlanEvidence{ID: 91, SHA256: sha256.Sum256([]byte("plan-a"))}
	persistedPlanHash := sha256.Sum256([]byte("plan-b"))
	persistedPlanID := uint64(92)

	mock.ExpectBegin()
	tx := root.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_provider_attempts` WHERE attempt_no = ? AND run_id = ? ORDER BY `ai_provider_attempts`.`id` LIMIT ?")).
		WithArgs(uint(1), int64(44), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "run_id", "attempt_no", "idempotency_key", "prepared_request_json",
			"prepared_request_sha256", "quote_json", "context_plan_id", "context_plan_sha256",
		}).AddRow(
			uint64(71), int64(44), uint(1), input.IdempotencyKey, input.PreparedRequestJSON,
			input.PreparedRequestSHA256[:], input.QuoteJSON, persistedPlanID, persistedPlanHash[:],
		))

	attempt, ok, err := repository.PreparePaidAttemptInTransaction(context.Background(), tx, input)
	if err == nil || ok || attempt != nil {
		t.Fatalf("context plan overwrite attempt=%+v ok=%v err=%v", attempt, ok, err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAttemptFirstDeltaUsesCompareAndSwap(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 28, 11, 0, 2, 123456000, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_provider_attempts` SET .* WHERE id = \\? AND first_delta_at IS NULL").
		WithArgs(now, now, uint64(71)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	recorded, err := repository.MarkAttemptFirstDelta(context.Background(), 71, now)
	if err != nil || !recorded {
		t.Fatalf("recorded=%v err=%v", recorded, err)
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
		PrepareStartedAt:      time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC),
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
