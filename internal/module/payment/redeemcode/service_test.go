package redeemcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"
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
		Transaction: &wallet.Transaction{ID: 9, TransactionNo: "WLT1", WalletID: 4, UserID: 7, Direction: wallet.DirectionIn, AmountCents: 250, BalanceBeforeCents: 100, BalanceAfterCents: 350, SourceType: wallet.SourceRedeemCode, SourceID: 8, IsDel: enum.CommonNo, CreatedAt: now},
		Wallet:      &wallet.Wallet{ID: 4, UserID: 7, BalanceCents: 500, TotalRechargeCents: 400, TotalConsumeCents: 20, IsDel: enum.CommonNo},
	}}
	service := NewService(repository)
	response, appErr := service.Redeem(context.Background(), 7, "zhr 2345 6789 abcd efgh jkmn")
	if appErr != nil || response.Amount != "2.50" || !response.Replayed || response.Transaction.TransactionNo != "WLT1" || response.Wallet.BalanceText != "5.00" {
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
		_, appErr := service.Redeem(context.Background(), 7, "ZHR-2345-6789-ABCD-EFGH-JKMN")
		if appErr == nil || appErr.Code != test.code {
			t.Fatalf("Redeem(%v) error=%+v", test.err, appErr)
		}
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

func (repository *fakeRepository) Redeem(context.Context, int64, string) (*RedemptionFact, error) {
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
