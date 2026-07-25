package aigateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

type testAssembler struct{ calls int }

func (a *testAssembler) AssembleAndQuote(context.Context, RunSnapshot, RunRequest) (PreparedCall, error) {
	a.calls++
	body := []byte(`{"model":"gpt-test","max_tokens":10}`)
	return PreparedCall{RequestBody: body, Quote: QuoteEvidence{PricingVersion: "v1", EffectiveMaxOutputTokens: 10, TargetHoldUnits: 25}}, nil
}

type testReserve struct {
	calls       int
	activeCalls int
	err         error
	activeErr   error
	facts       *LockedBillingFacts
}

type testTx struct{}

func (testTx) BillingTx() {}

type testTransactionRunner struct{}

func (testTransactionRunner) WithinTransaction(_ context.Context, fn func(Transaction) error) error {
	return fn(testTx{})
}

type testOwner struct{ err error }

func (o testOwner) EnsureRunnable(context.Context, Transaction, int64) error { return o.err }

type testRunStore struct{}

func (testRunStore) LoadRun(_ context.Context, runID int64) (RunSnapshot, error) {
	return RunSnapshot{RunID: runID, UserID: 7, RequestID: "r2", RequestFingerprint: [32]byte{2}}, nil
}
func (testRunStore) LockRunAndCharge(_ context.Context, _ Transaction, runID int64) (LockedRunCharge, error) {
	return LockedRunCharge{Run: RunSnapshot{RunID: runID}}, nil
}
func (testRunStore) RequestFingerprint(context.Context, int64, string) ([32]byte, error) {
	return [32]byte{}, nil
}

type persistedFingerprintRunStore struct{ fingerprint [32]byte }

func (s persistedFingerprintRunStore) LoadRun(context.Context, int64) (RunSnapshot, error) {
	return RunSnapshot{}, nil
}
func (s persistedFingerprintRunStore) LockRunAndCharge(context.Context, Transaction, int64) (LockedRunCharge, error) {
	return LockedRunCharge{}, nil
}
func (s persistedFingerprintRunStore) RequestFingerprint(context.Context, int64, string) ([32]byte, error) {
	return s.fingerprint, nil
}

type testAttemptStore struct {
	attempt       ProviderAttempt
	state         string
	terminal      DispatchResult
	recordErr     error
	preparedReads int
}

type testProvider struct{ calls int }

func (p *testProvider) Dispatch(context.Context, ProviderAttempt) (DispatchResult, error) {
	p.calls++
	return DispatchResult{ProviderRequestID: "provider-request-1", ResponseSHA256: sha256.Sum256([]byte("provider-response")), DispatchState: "dispatched", TerminalState: "succeeded", Usage: completeUsageForGatewayTest()}, nil
}

func completeUsageForGatewayTest() infraai.UsageSnapshot {
	return infraai.UsageSnapshot{Status: infraai.UsageStatusComplete, Items: []infraai.UsageItem{{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1}}}
}

func (s *testAttemptStore) PutPrepared(_ context.Context, _ Transaction, attempt ProviderAttempt) (PreparedWriteResult, error) {
	if s.state == "" {
		s.attempt = cloneAttempt(attempt)
		s.state = "prepared"
		return PreparedWriteResult{Attempt: cloneAttempt(s.attempt), Inserted: true}, nil
	}
	if s.state == "prepared" && sameAttemptEvidence(s.attempt, attempt) {
		return PreparedWriteResult{Attempt: cloneAttempt(s.attempt)}, nil
	}
	return PreparedWriteResult{}, ErrNotFound
}
func (s *testAttemptStore) GetPreparedForUpdate(context.Context, Transaction, int64, uint32) (ProviderAttempt, error) {
	s.preparedReads++
	if s.state != "prepared" {
		return ProviderAttempt{}, ErrNotFound
	}
	return cloneAttempt(s.attempt), nil
}
func (s *testAttemptStore) MarkDispatched(context.Context, Transaction, int64, uint32) (bool, error) {
	if s.state != "prepared" {
		return false, nil
	}
	s.state = "dispatched"
	return true, nil
}
func (s *testAttemptStore) GetDispatchedForUpdate(context.Context, Transaction, int64, uint32) (ProviderAttempt, error) {
	if s.state != "dispatched" {
		return ProviderAttempt{}, ErrNotFound
	}
	return cloneAttempt(s.attempt), nil
}
func (s *testAttemptStore) GetTerminalOutcome(context.Context, Transaction, int64, uint32) (DispatchResult, error) {
	if !validTerminalState(s.state) {
		return DispatchResult{}, ErrNotFound
	}
	return s.terminal, nil
}
func (s *testAttemptStore) RecordTerminalOutcome(_ context.Context, _ Transaction, _ int64, _ uint32, result DispatchResult) (TerminalOutcomeWriteResult, error) {
	if s.recordErr != nil {
		return TerminalOutcomeWriteResult{}, s.recordErr
	}
	if s.state != "dispatched" {
		return TerminalOutcomeWriteResult{}, ErrNotFound
	}
	s.state = result.TerminalState
	s.terminal = result
	return TerminalOutcomeWriteResult{Outcome: result}, nil
}

func testGatewayDependencies(reserve *testReserve, attempts *testAttemptStore) Dependencies {
	return Dependencies{Transactions: testTransactionRunner{}, Runs: testRunStore{}, Reserve: reserve, Attempts: attempts, Owner: testOwner{}}
}

func (r *testReserve) ReserveOrTopUp(_ context.Context, _ Transaction, runID int64, target int64) (LockedBillingFacts, error) {
	r.calls++
	if r.facts != nil {
		return *r.facts, r.err
	}
	return lockedFacts(runID, target), r.err
}
func (r *testReserve) EnsureActiveHold(_ context.Context, _ Transaction, runID int64, target int64) (LockedBillingFacts, error) {
	r.activeCalls++
	if r.facts != nil {
		return *r.facts, r.activeErr
	}
	return lockedFacts(runID, target), r.activeErr
}

func lockedFacts(runID, target int64) LockedBillingFacts {
	return LockedBillingFacts{RunID: runID, ChargeHeldUnits: target, ChargeHeldAuditMax: target, HoldTargetUnits: target, HoldActive: true}
}

func TestGatewayRejectsFingerprintConflictBeforeProvider(t *testing.T) {
	assembler := &testAssembler{}
	gateway := New(Dependencies{Assembler: assembler, Runs: immutableRunStore{snapshot: RunSnapshot{RunID: 1, UserID: 7, RequestID: "r1", RequestFingerprint: [32]byte{1}}}})
	request := RunRequest{RunID: 1, UserID: 7, RequestID: "r1", RequestFingerprint: [32]byte{1}}
	if _, err := gateway.AssembleAndQuote(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.RequestFingerprint = [32]byte{2}
	var got *Error
	if _, err := gateway.AssembleAndQuote(context.Background(), request); !errors.As(err, &got) || got.Code != ErrCodeFingerprintConflict {
		t.Fatalf("err=%v, want fingerprint conflict", err)
	}
	if assembler.calls != 1 {
		t.Fatalf("assembler calls=%d, want 1", assembler.calls)
	}
}

func TestGatewayRejectsPersistentFingerprintConflictAfterRestart(t *testing.T) {
	assembler := &testAssembler{}
	gateway := New(Dependencies{Assembler: assembler, Runs: persistedFingerprintRunStore{fingerprint: [32]byte{1}}})
	_, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 1, UserID: 7, RequestID: "r1", RequestFingerprint: [32]byte{2}})
	var got *Error
	if !errors.As(err, &got) || got.Code != ErrCodeFingerprintConflict {
		t.Fatalf("err=%v, want persisted fingerprint conflict", err)
	}
	if assembler.calls != 0 {
		t.Fatalf("assembler calls=%d, want 0", assembler.calls)
	}
}

func TestGatewayReservePersistsBeforeDispatchAndRecoversBytes(t *testing.T) {
	assembler := &testAssembler{}
	reserve := &testReserve{}
	store := &testAttemptStore{}
	deps := testGatewayDependencies(reserve, store)
	deps.Assembler = assembler
	gateway := New(deps)
	call, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 2, UserID: 7, RequestID: "r2", RequestFingerprint: [32]byte{2}})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 2, AttemptNo: 1, NewCall: &call})
	if err != nil || reserve.calls != 1 {
		t.Fatalf("attempt=%+v reserve_calls=%d err=%v", attempt, reserve.calls, err)
	}
	if err := gateway.MarkDispatched(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 2, AttemptNo: 1, NewCall: nil}); err == nil {
		t.Fatal("dispatched attempt must not be recovered as prepared")
	}
	// A fresh gateway with the same persisted evidence simulates a worker restart.
	recoveryStore := &testAttemptStore{attempt: attempt, state: "prepared"}
	prepared := New(testGatewayDependencies(&testReserve{}, recoveryStore))
	recovered, err := prepared.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 2, AttemptNo: 1})
	if err != nil || string(recovered.PreparedRequest) != string(attempt.PreparedRequest) || recovered.IdempotencyKey != attempt.IdempotencyKey {
		t.Fatalf("recovered=%+v err=%v", recovered, err)
	}
	if assembler.calls != 1 {
		t.Fatalf("prepared recovery assembled again: %d", assembler.calls)
	}
}

func TestGatewayPersistedPreparedRecoveryCanBeDispatched(t *testing.T) {
	store := &testAttemptStore{
		attempt: ProviderAttempt{RunID: 8, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`)},
		state:   "prepared",
	}
	gateway := New(testGatewayDependencies(&testReserve{}, store))
	attempt, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 8, AttemptNo: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.MarkDispatched(context.Background(), attempt); err != nil {
		t.Fatalf("persisted recovery should dispatch: %v", err)
	}
}

func TestGatewayDoesNotDispatchAnAlreadyDispatchedAttemptAgain(t *testing.T) {
	provider := &testProvider{}
	attempt := ProviderAttempt{RunID: 9, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`)}
	deps := testGatewayDependencies(&testReserve{}, &testAttemptStore{attempt: attempt, state: "prepared"})
	deps.Provider = provider
	gateway := New(deps)
	if _, err := gateway.Dispatch(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("dispatched replay must not call provider")
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d, want 1", provider.calls)
	}
}

func TestGatewayRequiresActiveHoldBeforeDispatch(t *testing.T) {
	attempt := ProviderAttempt{RunID: 13, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`), Quote: QuoteEvidence{TargetHoldUnits: 9}}
	reserve := &testReserve{activeErr: errors.New("hold inactive")}
	gateway := New(testGatewayDependencies(reserve, &testAttemptStore{attempt: attempt, state: "prepared"}))
	if err := gateway.MarkDispatched(context.Background(), attempt); err == nil {
		t.Fatal("expected inactive hold to block dispatch")
	}
	if reserve.activeCalls != 1 {
		t.Fatalf("active hold checks=%d, want 1", reserve.activeCalls)
	}
}

func TestGatewayFinalizeDefersSettlementAmountsToFinalizer(t *testing.T) {
	finalizer := &captureFinalizer{}
	err := New(Dependencies{Finalizer: finalizer}).Finalize(context.Background(), FinalizeInput{RunID: 14, ActualUnits: -1, HoldUnits: 0})
	if err != nil {
		t.Fatalf("gateway rejected caller amounts before finalizer: %v", err)
	}
	if finalizer.calls != 1 || finalizer.input.ActualUnits != -1 {
		t.Fatalf("finalizer was not given original input: %+v", finalizer)
	}
}

type captureFinalizer struct {
	calls int
	input FinalizeInput
}

func (f *captureFinalizer) Finalize(_ context.Context, input FinalizeInput) error {
	f.calls++
	f.input = input
	return nil
}

func TestGatewayRejectsReplayWithDifferentQuoteEvidence(t *testing.T) {
	gateway := New(testGatewayDependencies(&testReserve{}, &testAttemptStore{}))
	first := PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: QuoteEvidence{PricingVersion: "v1", EffectiveMaxOutputTokens: 16, TargetHoldUnits: 10}}
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 10, AttemptNo: 1, NewCall: &first}); err != nil {
		t.Fatal(err)
	}
	replay := first
	replay.Quote.EffectiveMaxOutputTokens = 17
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 10, AttemptNo: 1, NewCall: &replay}); err == nil {
		t.Fatal("replay with changed quote evidence must be rejected")
	}
}

func TestGatewayInsufficientBalanceCreatesNoAttempt(t *testing.T) {
	reserve := &testReserve{err: errors.New("insufficient balance")}
	gateway := New(testGatewayDependencies(reserve, &testAttemptStore{}))
	body := []byte(`{"model":"gpt-test"}`)
	call := PreparedCall{RequestBody: body, Quote: QuoteEvidence{TargetHoldUnits: 10}}
	_, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 3, AttemptNo: 1, NewCall: &call})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != ErrCodeInsufficientBalance {
		t.Fatalf("err=%v, want insufficient balance", err)
	}
	if store := gateway.deps.Attempts.(*testAttemptStore); store.state != "" {
		t.Fatal("insufficient reserve persisted an attempt")
	}
}

func TestGatewayRejectsReserveFactsThatDoNotMatchTheHoldTarget(t *testing.T) {
	facts := LockedBillingFacts{RunID: 16, ChargeHeldUnits: 9, ChargeHeldAuditMax: 9, HoldTargetUnits: 8, HoldActive: true}
	store := &testAttemptStore{}
	gateway := New(testGatewayDependencies(&testReserve{facts: &facts}, store))
	call := PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: QuoteEvidence{TargetHoldUnits: 8}}
	_, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 16, AttemptNo: 1, NewCall: &call})
	if err == nil {
		t.Fatal("inconsistent locked billing facts must be rejected")
	}
	if store.state != "" {
		t.Fatal("inconsistent reserve facts must not persist a prepared attempt")
	}
}

func TestGatewayRecoveryUsesPreparedReadForUpdate(t *testing.T) {
	attempt := ProviderAttempt{RunID: 17, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`)}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	if _, err := New(testGatewayDependencies(&testReserve{}, store)).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 17, AttemptNo: 1}); err != nil {
		t.Fatal(err)
	}
	if store.preparedReads != 1 {
		t.Fatalf("prepared strict reads=%d, want 1", store.preparedReads)
	}
}

func TestGatewayReturnsProviderAndOutcomePersistenceErrors(t *testing.T) {
	providerErr := errors.New("provider connection reset")
	recordErr := errors.New("attempt outcome write failed")
	attempt := ProviderAttempt{RunID: 11, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`)}
	deps := testGatewayDependencies(&testReserve{}, &testAttemptStore{attempt: attempt, state: "prepared", recordErr: recordErr})
	deps.Provider = providerError{err: providerErr}
	_, err := New(deps).Dispatch(context.Background(), attempt)
	if !errors.Is(err, providerErr) || !errors.Is(err, recordErr) {
		t.Fatalf("err=%v, want joined provider and persistence errors", err)
	}
}

func TestGatewayRejectsNonTerminalOutcome(t *testing.T) {
	attempt := ProviderAttempt{RunID: 15, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`)}
	store := &testAttemptStore{attempt: attempt, state: "dispatched"}
	err := New(testGatewayDependencies(&testReserve{}, store)).RecordOutcome(context.Background(), attempt, DispatchResult{
		DispatchState: infraai.DispatchStateDispatched,
		TerminalState: "dispatched",
		Usage:         completeUsageForGatewayTest(),
	})
	if err == nil {
		t.Fatal("non-terminal outcome must be rejected")
	}
}

func TestGatewayTerminalOutcomeReplayRequiresIdenticalEvidence(t *testing.T) {
	attempt := ProviderAttempt{RunID: 18, AttemptNo: 1, IdempotencyKey: "key", PreparedRequest: []byte(`{"model":"x"}`)}
	store := &testAttemptStore{attempt: attempt, state: "dispatched"}
	gateway := New(testGatewayDependencies(&testReserve{}, store))
	result := DispatchResult{ProviderRequestID: "provider-request-18", ResponseSHA256: sha256.Sum256([]byte("response-18")), DispatchState: infraai.DispatchStateDispatched, TerminalState: "succeeded", Usage: completeUsageForGatewayTest()}
	if err := gateway.RecordOutcome(context.Background(), attempt, result); err != nil {
		t.Fatal(err)
	}
	if err := gateway.RecordOutcome(context.Background(), attempt, result); err != nil {
		t.Fatalf("identical terminal replay failed: %v", err)
	}
	result.ProviderRequestID = "provider-request-different"
	if err := gateway.RecordOutcome(context.Background(), attempt, result); err == nil {
		t.Fatal("different terminal evidence must conflict")
	}
}

func TestGatewayAssembleValidatesPersistedRunIdentityBeforeAssembler(t *testing.T) {
	assembler := &testAssembler{}
	store := immutableRunStore{snapshot: RunSnapshot{RunID: 12, UserID: 7, RequestID: "stored-request", RequestFingerprint: [32]byte{1}}}
	gateway := New(Dependencies{Assembler: assembler, Runs: store})
	_, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 12, UserID: 7, RequestID: "caller-request", RequestFingerprint: [32]byte{1}})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != ErrCodeFingerprintConflict {
		t.Fatalf("err=%v, want persisted identity conflict", err)
	}
	if assembler.calls != 0 {
		t.Fatalf("assembler calls=%d, want 0", assembler.calls)
	}
}

type immutableRunStore struct{ snapshot RunSnapshot }

func (s immutableRunStore) LoadRun(context.Context, int64) (RunSnapshot, error) {
	return s.snapshot, nil
}
func (s immutableRunStore) LockRunAndCharge(context.Context, Transaction, int64) (LockedRunCharge, error) {
	return LockedRunCharge{Run: s.snapshot}, nil
}
func (s immutableRunStore) RequestFingerprint(context.Context, int64, string) ([32]byte, error) {
	return s.snapshot.RequestFingerprint, nil
}

type providerError struct{ err error }

func (p providerError) Dispatch(context.Context, ProviderAttempt) (DispatchResult, error) {
	return DispatchResult{}, p.err
}
