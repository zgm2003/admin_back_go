package redeemcode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"

	"github.com/redis/go-redis/v9"
)

func TestServiceGenerateCreatesCanonicalBatchAndUniqueCodes(t *testing.T) {
	now := time.Date(2026, 7, 24, 4, 0, 0, 987654321, time.FixedZone("CST", 8*60*60))
	expires := time.Date(2026, 7, 25, 12, 0, 0, 123456789, time.FixedZone("CST", 8*60*60))
	fakeClock := &countingClock{times: []time.Time{now}}
	repository := &fakeRepository{}
	repository.createFn = func(_ context.Context, record CreateBatchRecord) (*BatchWithCodes, bool, error) {
		if record.Batch.RequestFingerprintVersion != RequestFingerprintVersion || len(record.Batch.RequestFingerprint) != 64 {
			t.Fatalf("fingerprint fields=%+v", record.Batch)
		}
		canonical := `{"amount_cents":5000,"quantity":2,"expires_at":"2026-07-25T04:00:00.123456Z","note":"campaign"}`
		sum := sha256.Sum256([]byte(canonical))
		if record.Batch.RequestFingerprint != hex.EncodeToString(sum[:]) {
			t.Fatalf("fingerprint=%q", record.Batch.RequestFingerprint)
		}
		if record.Batch.ExpiresAt == nil || !record.Batch.ExpiresAt.Equal(time.Date(2026, 7, 25, 4, 0, 0, 123456000, time.UTC)) {
			t.Fatalf("expires_at=%v", record.Batch.ExpiresAt)
		}
		if record.Batch.Note != "campaign" || record.Batch.CreatedBy != 7 || record.Batch.Quantity != 2 || record.Batch.AmountCents != 5000 {
			t.Fatalf("record=%+v", record)
		}
		if len(record.Codes) != 2 || record.Codes[0].Code == record.Codes[1].Code {
			t.Fatalf("codes not unique: %+v", record.Codes)
		}
		record.Batch.ID = 11
		record.Batch.CreatedAt = now.UTC().Truncate(time.Microsecond)
		for index := range record.Codes {
			record.Codes[index].ID = int64(index + 21)
			record.Codes[index].BatchID = record.Batch.ID
		}
		return &BatchWithCodes{Batch: record.Batch, Codes: record.Codes}, false, nil
	}
	service := NewService(repository, WithClock(fakeClock), WithRandom(&incrementingReader{}))
	response, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{
		RequestID: "client.request:1", Amount: "50", Quantity: 2, ExpiresAt: &expires, Note: " campaign ",
	})
	if appErr != nil {
		t.Fatalf("GenerateBatch error=%+v", appErr)
	}
	if response.Batch.ID != 11 || response.Batch.Amount != "50.00" || response.Batch.Replayed || len(response.Codes) != 2 {
		t.Fatalf("response=%+v", response)
	}
	if fakeClock.calls != 1 {
		t.Fatalf("clock calls=%d", fakeClock.calls)
	}
}

func TestServiceGenerateReplaysBeforeRandomOrCurrentExpiryValidation(t *testing.T) {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	fingerprint, err := batchRequestFingerprint(100, 2, &expires, "old")
	if err != nil {
		t.Fatal(err)
	}
	existing := &BatchWithCodes{Batch: Batch{
		ID: 4, BatchNo: "RCB-old", RequestID: "request-1", RequestFingerprintVersion: RequestFingerprintVersion,
		RequestFingerprint: fingerprint, AmountCents: 100, Quantity: 2, ExpiresAt: &expires, Note: "old", CreatedBy: 7,
	}, Codes: []Code{{ID: 9, Code: "ZHR-2345-6789-ABCD-EFGH-JKMN"}, {ID: 8, Code: "ZHR-2345-6789-ABCD-EFGH-JKMP"}}}
	repository := &fakeRepository{findFn: func(context.Context, int64, string) (*BatchWithCodes, error) { return existing, nil }}
	service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })), WithRandom(errorReader{}))
	response, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "request-1", Amount: "1", Quantity: 2, ExpiresAt: &expires, Note: "old"})
	if appErr != nil {
		t.Fatalf("replay error=%+v", appErr)
	}
	if !response.Batch.Replayed || response.Codes[0].ID != 8 || response.Codes[1].ID != 9 || repository.createCalls != 0 {
		t.Fatalf("response=%+v createCalls=%d", response, repository.createCalls)
	}
}

func TestServiceGenerateRejectsVersionFingerprintAndReplayIntegrityConflicts(t *testing.T) {
	expires := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	fingerprint, _ := batchRequestFingerprint(100, 1, &expires, "")
	tests := []BatchWithCodes{
		{Batch: Batch{RequestFingerprintVersion: "old", RequestFingerprint: fingerprint, Quantity: 1}, Codes: []Code{{ID: 1}}},
		{Batch: Batch{RequestFingerprintVersion: RequestFingerprintVersion, RequestFingerprint: strings.Repeat("0", 64), Quantity: 1}, Codes: []Code{{ID: 1}}},
		{Batch: Batch{RequestFingerprintVersion: RequestFingerprintVersion, RequestFingerprint: fingerprint, Quantity: 1}, Codes: nil},
	}
	for index := range tests {
		repository := &fakeRepository{findResult: &tests[index]}
		service := NewService(repository, WithRandom(errorReader{}))
		_, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "request-1", Amount: "1", Quantity: 1, ExpiresAt: &expires})
		if appErr == nil {
			t.Fatalf("case %d error=nil", index)
		}
		want := ErrorRequestConflict
		if index == 2 {
			want = ErrorIntegrityViolation
		}
		if appErr.Code != want {
			t.Fatalf("case %d code=%q want %q", index, appErr.Code, want)
		}
	}
}

func TestServiceGenerateAllowsRequestRaceReplayButRejectsTrueExpiredCreate(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	expires := now.Add(-time.Second)
	repository := &fakeRepository{}
	repository.createFn = func(_ context.Context, record CreateBatchRecord) (*BatchWithCodes, bool, error) {
		return &BatchWithCodes{Batch: record.Batch, Codes: record.Codes}, true, nil
	}
	service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })), WithRandom(&incrementingReader{}))
	response, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "race", Amount: "1", Quantity: 1, ExpiresAt: &expires})
	if appErr != nil || !response.Batch.Replayed {
		t.Fatalf("race replay response=%+v error=%+v", response, appErr)
	}

	repository.createFn = func(context.Context, CreateBatchRecord) (*BatchWithCodes, bool, error) {
		return nil, false, ErrExpiryNotFuture
	}
	_, appErr = service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "new", Amount: "1", Quantity: 1, ExpiresAt: &expires})
	if appErr == nil || appErr.Code != ErrorRequestInvalid {
		t.Fatalf("expired create error=%+v", appErr)
	}
}

func TestServiceGenerateValidatesInputsAndBoundsCollisionRetries(t *testing.T) {
	control := rune(0)
	invalid := []struct {
		createdBy int64
		input     GenerateBatchInput
	}{
		{0, GenerateBatchInput{RequestID: "ok", Amount: "1", Quantity: 1}},
		{7, GenerateBatchInput{RequestID: " bad", Amount: "1", Quantity: 1}},
		{7, GenerateBatchInput{RequestID: strings.Repeat("a", 129), Amount: "1", Quantity: 1}},
		{7, GenerateBatchInput{RequestID: "ok", Amount: "0", Quantity: 1}},
		{7, GenerateBatchInput{RequestID: "ok", Amount: "1000000.01", Quantity: 1}},
		{7, GenerateBatchInput{RequestID: "ok", Amount: "1", Quantity: 0}},
		{7, GenerateBatchInput{RequestID: "ok", Amount: "1", Quantity: MaxBatchQuantity + 1}},
		{7, GenerateBatchInput{RequestID: "ok", Amount: "1", Quantity: 1, Note: "bad" + string(control)}},
		{7, GenerateBatchInput{RequestID: "ok", Amount: "1", Quantity: 1, Note: strings.Repeat("界", 256)}},
	}
	for index, test := range invalid {
		repository := &fakeRepository{}
		service := NewService(repository)
		_, appErr := service.GenerateBatch(context.Background(), test.createdBy, test.input)
		if appErr == nil || appErr.Code != ErrorRequestInvalid || repository.findCalls != 0 {
			t.Fatalf("case %d error=%+v findCalls=%d", index, appErr, repository.findCalls)
		}
	}

	repository := &fakeRepository{createErr: ErrCodeCollision}
	service := NewService(repository, WithRandom(&incrementingReader{}))
	_, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "collision", Amount: "1", Quantity: 1})
	if appErr == nil || appErr.Code != ErrorIntegrityViolation || repository.createCalls != maxCreateBatchAttempts {
		t.Fatalf("collision error=%+v calls=%d", appErr, repository.createCalls)
	}
}

func TestServiceRepositoryIntegrityErrorsArePermanentAndNonRetryable(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	dependencyErr := errors.New("database unavailable")
	tests := []struct {
		name   string
		setErr func(*fakeRepository, error)
		invoke func(*fakeRepository) *apperror.Error
	}{
		{name: "generate_find_batch", setErr: func(repository *fakeRepository, err error) { repository.findErr = err }, invoke: func(repository *fakeRepository) *apperror.Error {
			service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })), WithRandom(&incrementingReader{}))
			_, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "request-1", Amount: "1", Quantity: 1, ExpiresAt: &expires})
			return appErr
		}},
		{name: "list", setErr: func(repository *fakeRepository, err error) { repository.listErr = err }, invoke: func(repository *fakeRepository) *apperror.Error {
			service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })))
			_, appErr := service.List(context.Background(), ListQuery{})
			return appErr
		}},
		{name: "lookup", setErr: func(repository *fakeRepository, err error) { repository.lookupErr = err }, invoke: func(repository *fakeRepository) *apperror.Error {
			service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })))
			_, appErr := service.Lookup(context.Background(), LookupInput{Code: "ZHR-2345-6789-ABCD-EFGH-JKMN"})
			return appErr
		}},
		{name: "export", setErr: func(repository *fakeRepository, err error) { repository.exportErr = err }, invoke: func(repository *fakeRepository) *apperror.Error {
			service := NewService(repository, WithClock(clock.Func(func() time.Time { return now })))
			_, appErr := service.Export(context.Background(), ExportInput{})
			return appErr
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			test.setErr(repository, ErrIntegrityViolation)
			appErr := test.invoke(repository)
			if appErr == nil || appErr.Code != ErrorIntegrityViolation || appErr.Category != apperror.CategoryInternal || appErr.HTTPStatus != http.StatusInternalServerError || appErr.Retryable() || appErr.Cause != nil {
				t.Fatalf("integrity error=%+v", appErr)
			}

			repository = &fakeRepository{}
			test.setErr(repository, dependencyErr)
			appErr = test.invoke(repository)
			if appErr == nil || appErr.Code != ErrorDependencyUnavailable || appErr.Category != apperror.CategoryDependency || appErr.HTTPStatus != http.StatusServiceUnavailable || !appErr.Retryable() || !errors.Is(appErr, dependencyErr) {
				t.Fatalf("dependency error=%+v", appErr)
			}
		})
	}
}

func TestServiceGenerateFindBatchIntegrityRecordsIntegrityReason(t *testing.T) {
	recorder := &captureRecorder{}
	repository := &fakeRepository{findErr: ErrIntegrityViolation}
	service := NewService(repository, WithTelemetry(recorder), WithRandom(&incrementingReader{}))
	_, appErr := service.GenerateBatch(context.Background(), 7, GenerateBatchInput{RequestID: "request-1", Amount: "1", Quantity: 1})
	if appErr == nil || appErr.Code != ErrorIntegrityViolation {
		t.Fatalf("GenerateBatch error=%+v", appErr)
	}
	assertCapturedMetric(t, recorder.events, "payment.redeem_code.batches", map[string]any{
		"operation": "generate", "outcome": "error", "reason": "integrity",
	})
}

func TestServiceListLookupProjectsDerivedStateAndCapturesOneNow(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 123456789, time.UTC)
	expires := now.Add(-time.Microsecond)
	repository := &fakeRepository{}
	repository.listRows = []CodeView{{ID: 1, Code: "ZHR-2345-6789-ABCD-EFGH-JKMN", State: StateUnused, ExpiresAt: &expires, AmountCents: 123}}
	repository.lookupResult = &repository.listRows[0]
	fakeClock := &countingClock{times: []time.Time{now, now}}
	service := NewService(repository, WithClock(fakeClock))
	list, appErr := service.List(context.Background(), ListQuery{CurrentPage: -1, PageSize: 1000, BatchNo: " batch "})
	if appErr != nil || len(list.List) != 1 || list.List[0].State != StateExpired || list.List[0].Amount != "1.23" {
		t.Fatalf("list=%+v error=%+v", list, appErr)
	}
	if repository.listQuery.CurrentPage != 1 || repository.listQuery.PageSize != 100 || repository.listQuery.BatchNo != "batch" || !repository.listNow.Equal(now.Truncate(time.Microsecond)) {
		t.Fatalf("normalized query=%+v now=%v", repository.listQuery, repository.listNow)
	}
	lookup, appErr := service.Lookup(context.Background(), LookupInput{Code: " zhr 2345 6789 abcd efgh jkmn "})
	if appErr != nil || lookup.Item == nil || repository.lookupCode != repository.listRows[0].Code {
		t.Fatalf("lookup=%+v error=%+v code=%q", lookup, appErr, repository.lookupCode)
	}
	if fakeClock.calls != 2 {
		t.Fatalf("clock calls=%d", fakeClock.calls)
	}
}

func TestServiceVoidDeduplicatesSortsAndRejectsInvalidSets(t *testing.T) {
	repository := &fakeRepository{voidCount: 3}
	service := NewService(repository)
	response, appErr := service.Void(context.Background(), VoidInput{IDs: []int64{3, 1, 3, 2}})
	if appErr != nil || response.Voided != 3 || strings.Trim(strings.Join(int64Strings(repository.voidIDs), ","), ",") != "1,2,3" {
		t.Fatalf("response=%+v error=%+v ids=%v", response, appErr, repository.voidIDs)
	}
	for _, ids := range [][]int64{nil, {0}, make([]int64, MaxVoidCodes+1)} {
		if _, appErr := service.Void(context.Background(), VoidInput{IDs: ids}); appErr == nil || appErr.Code != ErrorRequestInvalid {
			t.Fatalf("Void(%d) error=%+v", len(ids), appErr)
		}
	}
}

func TestExportIsBoundedAndReturnsSynchronousCSV(t *testing.T) {
	repository := &fakeRepository{}
	repository.exportRows = make([]CodeView, MaxExportRows+1)
	service := NewService(repository, WithClock(clock.Func(func() time.Time {
		return time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	})))
	if _, appErr := service.Export(context.Background(), ExportInput{}); appErr == nil || appErr.Code != ErrorExportTooLarge {
		t.Fatalf("large export error=%+v", appErr)
	}
	if repository.exportLimit != MaxExportRows+1 {
		t.Fatalf("export limit=%d", repository.exportLimit)
	}

	repository.exportRows = []CodeView{{ID: 1, Code: "ZHR-2345-6789-ABCD-EFGH-JKMN", BatchNo: "RCB1", AmountCents: 100, State: StateUnused, Note: "=cmd"}}
	response, appErr := service.Export(context.Background(), ExportInput{})
	if appErr != nil || response.RowCount != 1 || response.Filename != "redeem-codes-20260724.csv" || !strings.HasPrefix(response.Content, "\ufeff") || !strings.Contains(response.Content, "'=cmd") {
		t.Fatalf("export=%+v error=%+v", response, appErr)
	}
}

func TestServiceRedeemMapsFactsAndStableDomainErrors(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repository := &fakeRepository{redeemFact: &RedemptionFact{
		AmountCents: 250, Replayed: true,
		Transaction: &wallet.Transaction{ID: 9, TransactionNo: "WLT1", WalletID: 4, UserID: 7, Direction: wallet.DirectionIn, AmountUnits: 250_000_000, BalanceBeforeUnits: 100_000_000, BalanceAfterUnits: 350_000_000, SourceType: wallet.SourceRedeemCode, SourceID: 8, IsDel: enum.CommonNo, CreatedAt: now},
		Wallet:      &wallet.Wallet{ID: 4, UserID: 7, BalanceUnits: 500_000_000, TotalRechargeUnits: 400_000_000, TotalConsumeUnits: 20_000_000, IsDel: enum.CommonNo},
	}}
	service := NewService(repository, WithAttemptLimiter(newAllowAttemptLimiter()))
	response, appErr := service.Redeem(context.Background(), 7, "admin", "zhr 2345 6789 abcd efgh jkmn")
	if appErr != nil || response.Amount != "2.5" || !response.Replayed || response.Transaction.TransactionNo != "WLT1" || response.Wallet.Balance != "5" {
		t.Fatalf("response=%+v error=%+v", response, appErr)
	}

	tests := []struct {
		err  error
		code string
	}{
		{ErrUnavailable, ErrorWalletUnavailable},
		{ErrIntegrityViolation, ErrorWalletIntegrityViolation},
		{errors.New("database unavailable"), ErrorWalletDependencyUnavailable},
	}
	for _, test := range tests {
		repository.redeemFact = nil
		repository.redeemErr = test.err
		_, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
		if appErr == nil || appErr.Code != test.code {
			t.Fatalf("Redeem(%v) error=%+v", test.err, appErr)
		}
	}
}

func TestServiceRedeemFailsClosedWithoutLimiter(t *testing.T) {
	repository := &fakeRepository{redeemFn: func(context.Context, int64, string) (*RedemptionFact, error) {
		t.Fatal("repository must not be called")
		return nil, nil
	}}
	_, appErr := NewService(repository).Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.HTTPStatus != http.StatusServiceUnavailable || appErr.Code != ErrorWalletRateLimitUnavailable || appErr.Category != apperror.CategoryDependency || appErr.Retry != apperror.Retryable {
		t.Fatalf("error=%+v", appErr)
	}
}

func TestServiceRedeemLogsReleaseFailureWithoutSensitiveFields(t *testing.T) {
	var logs bytes.Buffer
	limiter := newAllowAttemptLimiter()
	limiter.releaseFn = func(context.Context, AttemptLease) error { return errors.New("redis raw secret") }
	service := NewService(&fakeRepository{redeemFact: validTelemetryRedemptionFact()}, WithAttemptLimiter(limiter), WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	response, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr != nil || response == nil {
		t.Fatalf("response=%+v error=%+v", response, appErr)
	}
	line := logs.String()
	if !strings.Contains(line, "operation=redeem") || !strings.Contains(line, "reason=release") || strings.Contains(line, "redis raw secret") || strings.Contains(line, "user_id") {
		t.Fatalf("controlled log=%q", line)
	}
}

func TestServiceRedeemRateLimitChecksBeforeRepositoryAndRecordsUserFailures(t *testing.T) {
	var calls []string
	limiter := &fakeAttemptLimiter{}
	limiter.acquireFn = func(context.Context, string, int64) (AttemptLease, error) {
		calls = append(calls, "acquire")
		return AttemptLease{Owner: "owner"}, nil
	}
	limiter.failureStateFn = func(ctx context.Context, _ string, _ int64) (FailureState, error) {
		calls = append(calls, "state")
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > attemptTimeout || time.Until(deadline) < attemptTimeout-time.Second {
			t.Fatalf("attempt deadline=%v ok=%t", deadline, ok)
		}
		return FailureState{}, nil
	}
	limiter.recordFailureFn = func(ctx context.Context, _ string, _ int64) (FailureState, error) {
		calls = append(calls, "record")
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("cleanup context has no deadline")
		}
		return FailureState{Count: 1, TTL: failureWindow}, nil
	}
	limiter.releaseFn = func(ctx context.Context, lease AttemptLease) error {
		calls = append(calls, "release")
		if lease.Owner != "owner" {
			t.Fatalf("lease=%+v", lease)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("cleanup context has no deadline")
		}
		return nil
	}
	repository := &fakeRepository{redeemErr: ErrUnavailable}
	service := NewService(repository, WithAttemptLimiter(limiter))
	_, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.Code != ErrorWalletUnavailable {
		t.Fatalf("error=%+v", appErr)
	}
	if strings.Join(calls, ",") != "acquire,state,record,release" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestServiceRedeemRateLimitRejectsBeforeRepository(t *testing.T) {
	limiter := &fakeAttemptLimiter{
		acquireFn: func(context.Context, string, int64) (AttemptLease, error) { return AttemptLease{Owner: "owner"}, nil },
		failureStateFn: func(context.Context, string, int64) (FailureState, error) {
			return FailureState{Count: failureLimit, TTL: 3 * time.Second}, nil
		},
		recordFailureFn: func(context.Context, string, int64) (FailureState, error) {
			t.Fatal("RecordFailure called")
			return FailureState{}, nil
		},
		releaseFn: func(context.Context, AttemptLease) error { return nil },
	}
	repository := &fakeRepository{redeemFn: func(context.Context, int64, string) (*RedemptionFact, error) {
		t.Fatal("Redeem called")
		return nil, nil
	}}
	service := NewService(repository, WithAttemptLimiter(limiter))
	_, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.HTTPStatus != http.StatusTooManyRequests || appErr.Category != apperror.CategoryRateLimit || appErr.TemplateData["retry_after"] != 3 {
		t.Fatalf("error=%+v", appErr)
	}
}

func TestServiceRedeemFailureThresholdBoundary(t *testing.T) {
	count := 0
	repositoryCalls := 0
	limiter := newAllowAttemptLimiter()
	limiter.failureStateFn = func(context.Context, string, int64) (FailureState, error) {
		return FailureState{Count: count, TTL: failureWindow}, nil
	}
	limiter.recordFailureFn = func(context.Context, string, int64) (FailureState, error) {
		count++
		return FailureState{Count: count, TTL: failureWindow}, nil
	}
	repository := &fakeRepository{redeemFn: func(context.Context, int64, string) (*RedemptionFact, error) {
		repositoryCalls++
		return nil, ErrUnavailable
	}}
	service := NewService(repository, WithAttemptLimiter(limiter))
	for attempt := 1; attempt <= failureLimit; attempt++ {
		_, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
		if appErr == nil || appErr.Code != ErrorWalletUnavailable || appErr.HTTPStatus != http.StatusBadRequest {
			t.Fatalf("attempt %d error=%+v", attempt, appErr)
		}
	}
	if count != failureLimit || repositoryCalls != failureLimit {
		t.Fatalf("count=%d repositoryCalls=%d", count, repositoryCalls)
	}
	_, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.HTTPStatus != http.StatusTooManyRequests || repositoryCalls != failureLimit {
		t.Fatalf("threshold error=%+v repositoryCalls=%d", appErr, repositoryCalls)
	}
}

func TestServiceRedeemCancellationStillCleansUpWithIndependentContext(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	recorded := make(chan bool, 1)
	released := make(chan bool, 1)
	limiter := newAllowAttemptLimiter()
	limiter.recordFailureFn = func(ctx context.Context, _ string, _ int64) (FailureState, error) {
		recorded <- ctx.Err() == nil
		return FailureState{Count: 1, TTL: failureWindow}, nil
	}
	limiter.releaseFn = func(ctx context.Context, _ AttemptLease) error {
		released <- ctx.Err() == nil
		return nil
	}
	repository := &fakeRepository{redeemFn: func(ctx context.Context, _ int64, _ string) (*RedemptionFact, error) {
		close(entered)
		<-ctx.Done()
		return nil, ErrUnavailable
	}}
	service := NewService(repository, WithAttemptLimiter(limiter))
	done := make(chan *apperror.Error, 1)
	go func() {
		_, appErr := service.Redeem(requestCtx, 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
		done <- appErr
	}()
	<-entered
	cancel()
	if appErr := <-done; appErr == nil || appErr.Code != ErrorWalletUnavailable {
		t.Fatalf("error=%+v", appErr)
	}
	if ok := <-recorded; !ok {
		t.Fatal("RecordFailure cleanup context was canceled")
	}
	if ok := <-released; !ok {
		t.Fatal("Release cleanup context was canceled")
	}
}

func TestServiceRedeemInvalidInputsRecordFailureInsideLease(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
		want string
	}{
		{name: "empty", code: "   ", want: ErrorWalletCodeRequired},
		{name: "malformed", code: "not-a-code", want: ErrorWalletUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			limiter := &fakeAttemptLimiter{
				acquireFn: func(context.Context, string, int64) (AttemptLease, error) {
					calls = append(calls, "acquire")
					return AttemptLease{Owner: "owner"}, nil
				},
				failureStateFn: func(context.Context, string, int64) (FailureState, error) {
					calls = append(calls, "state")
					return FailureState{}, nil
				},
				recordFailureFn: func(context.Context, string, int64) (FailureState, error) {
					calls = append(calls, "record")
					return FailureState{Count: 1, TTL: 10 * time.Minute}, nil
				},
				releaseFn: func(context.Context, AttemptLease) error { calls = append(calls, "release"); return nil },
			}
			repository := &fakeRepository{redeemFn: func(context.Context, int64, string) (*RedemptionFact, error) {
				t.Fatal("repository called for invalid input")
				return nil, nil
			}}
			_, appErr := NewService(repository, WithAttemptLimiter(limiter)).Redeem(context.Background(), 7, "admin", test.code)
			if appErr == nil || appErr.Code != test.want || strings.Join(calls, ",") != "acquire,state,record,release" {
				t.Fatalf("error=%+v calls=%v", appErr, calls)
			}
		})
	}
}

func TestServiceRedeemSuccessAndReplayDoNotRecordFailure(t *testing.T) {
	for _, replayed := range []bool{false, true} {
		t.Run(fmt.Sprintf("replayed=%t", replayed), func(t *testing.T) {
			records := 0
			limiter := newAllowAttemptLimiter()
			limiter.recordFailureFn = func(context.Context, string, int64) (FailureState, error) { records++; return FailureState{}, nil }
			fact := validTelemetryRedemptionFact()
			fact.Replayed = replayed
			response, appErr := NewService(&fakeRepository{redeemFact: fact}, WithAttemptLimiter(limiter)).Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
			if appErr != nil || response == nil || response.Replayed != replayed || records != 0 {
				t.Fatalf("response=%+v error=%+v records=%d", response, appErr, records)
			}
		})
	}
}

func TestServiceRedeemLimiterAndDependencyFailuresDoNotRecord(t *testing.T) {
	tests := []struct {
		name         string
		configure    func(*fakeAttemptLimiter)
		repository   *fakeRepository
		wantCode     string
		wantCategory apperror.Category
		wantRetry    apperror.RetryClass
	}{
		{name: "acquire", configure: func(l *fakeAttemptLimiter) {
			l.acquireFn = func(context.Context, string, int64) (AttemptLease, error) {
				return AttemptLease{}, errors.New("redis down")
			}
		}, repository: &fakeRepository{}, wantCode: ErrorWalletRateLimitUnavailable, wantCategory: apperror.CategoryDependency, wantRetry: apperror.Retryable},
		{name: "state", configure: func(l *fakeAttemptLimiter) {
			l.failureStateFn = func(context.Context, string, int64) (FailureState, error) {
				return FailureState{}, errors.New("redis down")
			}
		}, repository: &fakeRepository{}, wantCode: ErrorWalletRateLimitUnavailable, wantCategory: apperror.CategoryDependency, wantRetry: apperror.Retryable},
		{name: "mysql", configure: func(l *fakeAttemptLimiter) {}, repository: &fakeRepository{redeemErr: errors.New("mysql down")}, wantCode: ErrorWalletDependencyUnavailable, wantCategory: apperror.CategoryDependency, wantRetry: apperror.Retryable},
		{name: "record", configure: func(l *fakeAttemptLimiter) {
			l.recordFailureFn = func(context.Context, string, int64) (FailureState, error) {
				return FailureState{}, errors.New("redis down")
			}
		}, repository: &fakeRepository{redeemErr: ErrUnavailable}, wantCode: ErrorWalletRateLimitUnavailable, wantCategory: apperror.CategoryDependency, wantRetry: apperror.Retryable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			records := 0
			limiter := newAllowAttemptLimiter()
			originalRecord := limiter.recordFailureFn
			limiter.recordFailureFn = func(ctx context.Context, platform string, userID int64) (FailureState, error) {
				records++
				return originalRecord(ctx, platform, userID)
			}
			test.configure(limiter)
			if test.name == "record" {
				failure := limiter.recordFailureFn
				limiter.recordFailureFn = func(ctx context.Context, platform string, userID int64) (FailureState, error) {
					records++
					return failure(ctx, platform, userID)
				}
			}
			_, appErr := NewService(test.repository, WithAttemptLimiter(limiter)).Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
			wantRecords := 0
			if test.name == "record" {
				wantRecords = 1
			}
			if appErr == nil || appErr.HTTPStatus != http.StatusServiceUnavailable || appErr.Code != test.wantCode || appErr.Category != test.wantCategory || appErr.Retry != test.wantRetry || records != wantRecords {
				t.Fatalf("error=%+v records=%d", appErr, records)
			}
		})
	}
}

func TestServiceRedeemUserFailureReleaseFailureFailsClosed(t *testing.T) {
	limiter := newAllowAttemptLimiter()
	limiter.releaseFn = func(context.Context, AttemptLease) error { return errors.New("redis down") }
	_, appErr := NewService(&fakeRepository{redeemErr: ErrUnavailable}, WithAttemptLimiter(limiter)).Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.HTTPStatus != http.StatusServiceUnavailable || appErr.Code != ErrorWalletRateLimitUnavailable || appErr.Category != apperror.CategoryDependency || appErr.Retry != apperror.Retryable {
		t.Fatalf("error=%+v", appErr)
	}
}

func TestServiceRedeemLockedReturnsIntegerRetryAfterWithoutRepository(t *testing.T) {
	limiter := newAllowAttemptLimiter()
	limiter.acquireFn = func(context.Context, string, int64) (AttemptLease, error) {
		return AttemptLease{}, &AttemptLockedError{RetryAfter: 2}
	}
	repository := &fakeRepository{redeemFn: func(context.Context, int64, string) (*RedemptionFact, error) {
		t.Fatal("repository called")
		return nil, nil
	}}
	_, appErr := NewService(repository, WithAttemptLimiter(limiter)).Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.HTTPStatus != http.StatusTooManyRequests || appErr.TemplateData["retry_after"] != 2 {
		t.Fatalf("error=%+v", appErr)
	}
}

func TestServiceRedeemPassesProvidedPlatformToAttemptLimiter(t *testing.T) {
	var platforms []string
	limiter := newAllowAttemptLimiter()
	limiter.acquireFn = func(_ context.Context, platform string, _ int64) (AttemptLease, error) {
		platforms = append(platforms, platform)
		return AttemptLease{Owner: "owner"}, nil
	}
	limiter.failureStateFn = func(_ context.Context, platform string, _ int64) (FailureState, error) {
		platforms = append(platforms, platform)
		return FailureState{}, nil
	}
	limiter.recordFailureFn = func(_ context.Context, platform string, _ int64) (FailureState, error) {
		platforms = append(platforms, platform)
		return FailureState{Count: 1, TTL: failureWindow}, nil
	}
	service := NewService(&fakeRepository{}, WithAttemptLimiter(limiter))
	response, appErr := service.Redeem(context.Background(), 7, "ios", "not-a-code")
	if response != nil || appErr == nil || strings.Join(platforms, ",") != "ios,ios,ios" {
		t.Fatalf("response=%+v error=%+v platforms=%v", response, appErr, platforms)
	}
}

func TestServiceRedeemTypedNilLimiterMapsRateLimitUnavailable(t *testing.T) {
	var client *redis.Client
	service := NewService(&fakeRepository{}, WithAttemptLimiter(NewRedisAttemptLimiter(client)))
	_, appErr := service.Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.Code != ErrorWalletRateLimitUnavailable || appErr.Category != apperror.CategoryDependency || appErr.HTTPStatus != http.StatusServiceUnavailable || appErr.Retry != apperror.Retryable {
		t.Fatalf("error=%+v", appErr)
	}
}

func TestWalletRateLimitedUsesStableCodeAndMessageID(t *testing.T) {
	appErr := walletRateLimited(3)
	if appErr.Code != "wallet.redeem.rate_limited" || appErr.MessageID != "wallet.redeem.rate_limited" || appErr.Category != apperror.CategoryRateLimit || appErr.HTTPStatus != http.StatusTooManyRequests || appErr.Retry != apperror.Retryable || appErr.TemplateData["retry_after"] != 3 {
		t.Fatalf("error=%+v", appErr)
	}
}

func TestServiceRedeemPreservesWrappedLockRetryAfter(t *testing.T) {
	limiter := newAllowAttemptLimiter()
	limiter.acquireFn = func(context.Context, string, int64) (AttemptLease, error) {
		return AttemptLease{}, fmt.Errorf("acquire: %w", &AttemptLockedError{RetryAfter: 4})
	}
	repository := &fakeRepository{redeemFn: func(context.Context, int64, string) (*RedemptionFact, error) {
		t.Fatal("repository called")
		return nil, nil
	}}
	_, appErr := NewService(repository, WithAttemptLimiter(limiter)).Redeem(context.Background(), 7, "admin", "ZHR-2345-6789-ABCD-EFGH-JKMN")
	if appErr == nil || appErr.HTTPStatus != http.StatusTooManyRequests || appErr.TemplateData["retry_after"] != 4 {
		t.Fatalf("error=%+v", appErr)
	}
}

type fakeRepository struct {
	findFn       func(context.Context, int64, string) (*BatchWithCodes, error)
	findResult   *BatchWithCodes
	findErr      error
	findCalls    int
	createFn     func(context.Context, CreateBatchRecord) (*BatchWithCodes, bool, error)
	createErr    error
	createCalls  int
	listRows     []CodeView
	listTotal    int64
	listErr      error
	listQuery    ListQuery
	listNow      time.Time
	lookupResult *CodeView
	lookupErr    error
	lookupCode   string
	exportRows   []CodeView
	exportErr    error
	exportLimit  int
	voidCount    int
	voidErr      error
	voidIDs      []int64
	redeemFact   *RedemptionFact
	redeemErr    error
	redeemFn     func(context.Context, int64, string) (*RedemptionFact, error)
}

func (repository *fakeRepository) FindBatchByRequest(ctx context.Context, userID int64, requestID string) (*BatchWithCodes, error) {
	repository.findCalls++
	if repository.findFn != nil {
		return repository.findFn(ctx, userID, requestID)
	}
	return repository.findResult, repository.findErr
}

func (repository *fakeRepository) CreateOrReplayBatch(ctx context.Context, record CreateBatchRecord) (*BatchWithCodes, bool, error) {
	repository.createCalls++
	if repository.createFn != nil {
		return repository.createFn(ctx, record)
	}
	return nil, false, repository.createErr
}

func (repository *fakeRepository) ListCodes(_ context.Context, query ListQuery, now time.Time) ([]CodeView, int64, error) {
	repository.listQuery, repository.listNow = query, now
	return repository.listRows, repository.listTotal, repository.listErr
}

func (repository *fakeRepository) LookupCode(_ context.Context, code string, _ time.Time) (*CodeView, error) {
	repository.lookupCode = code
	return repository.lookupResult, repository.lookupErr
}

func (repository *fakeRepository) ExportCodes(_ context.Context, _ ListQuery, _ time.Time, limit int) ([]CodeView, error) {
	repository.exportLimit = limit
	return repository.exportRows, repository.exportErr
}

func (repository *fakeRepository) VoidCodes(_ context.Context, ids []int64, _ time.Time) (int, error) {
	repository.voidIDs = append([]int64(nil), ids...)
	return repository.voidCount, repository.voidErr
}

func (repository *fakeRepository) Redeem(ctx context.Context, userID int64, code string) (*RedemptionFact, error) {
	if repository.redeemFn != nil {
		return repository.redeemFn(ctx, userID, code)
	}
	return repository.redeemFact, repository.redeemErr
}

type countingClock struct {
	times []time.Time
	calls int
}

func (fake *countingClock) Now() time.Time {
	index := fake.calls
	fake.calls++
	if len(fake.times) == 0 {
		return time.Time{}
	}
	if index >= len(fake.times) {
		return fake.times[len(fake.times)-1]
	}
	return fake.times[index]
}

type incrementingReader struct{ next byte }

func (reader *incrementingReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = reader.next
		reader.next = (reader.next + 1) % 248
	}
	return len(buffer), nil
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func int64Strings(values []int64) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(rune('0' + value))
	}
	return result
}
