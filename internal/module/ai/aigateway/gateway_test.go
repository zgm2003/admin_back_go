package aigateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/apperror"
)

type testAssembler struct{ calls int }

func (a *testAssembler) AssembleAndQuote(context.Context, RunSnapshot, RunRequest) (PreparedCall, error) {
	a.calls++
	body := []byte(`{"model":"gpt-test","max_tokens":10}`)
	return PreparedCall{RequestBody: body, Quote: validQuote(25)}, nil
}

func validQuote(target int64) QuoteEvidence {
	return QuoteEvidence{PricingVersion: "v1", EffectiveMaxOutputTokens: 10, CurrentCallMaxUnits: target, TargetHoldUnits: target, UpperBoundItems: []billing.UsageItem{{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}}}
}

func quoteWithPreparedRequestHash(quote QuoteEvidence, hash [32]byte) QuoteEvidence {
	quote.PreparedRequestSHA256 = hash
	return quote
}

func requestIdentity(text string) requestidentity.Input {
	return requestidentity.Input{UserID: 7, Operation: "generate", Modality: "text", AgentID: 1, ModelID: "model", NormalizedText: text, Options: requestidentity.GenerationOptions{MaxOutputTokens: 10}}
}

func requestFingerprint(identity requestidentity.Input) [32]byte {
	fingerprint, err := requestidentity.Fingerprint(identity)
	if err != nil {
		panic(err)
	}
	return fingerprint
}

func sealTestCall(runID int64, fingerprint [32]byte, call PreparedCall) PreparedCall {
	call.Quote.RequestFingerprint = fingerprint
	canonical, err := canonicalPrepared(call)
	if err != nil {
		panic(err)
	}
	canonical.assemblyRunID = runID
	canonical.assemblyFingerprint = fingerprint
	canonical.assemblySeal = preparedAssemblySeal(canonical, runID, fingerprint)
	return canonical
}

func TestCanonicalPreparedBindsQuoteToExactRequest(t *testing.T) {
	body := []byte(`{"model":"gpt-test"}`)
	canonical, err := canonicalPrepared(PreparedCall{RequestBody: body, Quote: validQuote(5)})
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		PreparedRequestSHA256 [32]byte `json:"prepared_request_sha256"`
	}
	if err := json.Unmarshal([]byte(quoteJSON(canonical.Quote)), &evidence); err != nil {
		t.Fatal(err)
	}
	if want := sha256.Sum256(body); evidence.PreparedRequestSHA256 != want {
		t.Fatalf("quote request hash=%x, want %x", evidence.PreparedRequestSHA256, want)
	}
}

func TestCanonicalPreparedRejectsQuoteBoundToDifferentRequest(t *testing.T) {
	body := []byte(`{"model":"gpt-test"}`)
	quote := quoteWithPreparedRequestHash(validQuote(5), sha256.Sum256([]byte(`{"model":"other"}`)))
	if _, err := canonicalPrepared(PreparedCall{RequestBody: body, Quote: quote}); err == nil {
		t.Fatal("prepared call accepted quote bound to different request bytes")
	}
}

func TestCanonicalPreparedRejectsInvalidContextPlanEvidence(t *testing.T) {
	body := []byte(`{"model":"gpt-test"}`)
	for _, plan := range []*ContextPlanEvidence{
		{SHA256: sha256.Sum256([]byte("plan"))},
		{ID: 91},
	} {
		if _, err := canonicalPrepared(PreparedCall{RequestBody: body, ContextPlan: plan, Quote: validQuote(5)}); err == nil {
			t.Fatalf("invalid context plan was accepted: %+v", plan)
		}
	}
}

func validAttempt(runID int64, attemptNo uint32, target int64) ProviderAttempt {
	body := []byte(`{"model":"x"}`)
	hash := sha256.Sum256(body)
	quote := validQuote(target)
	quote.PreparedRequestSHA256 = hash
	return ProviderAttempt{RunID: runID, AttemptNo: attemptNo, IdempotencyKey: attemptKey(runID, attemptNo), PreparedRequest: body, RequestSHA256: hash, Quote: quote}
}

func TestSameAttemptEvidenceIncludesContextPlan(t *testing.T) {
	planHash := sha256.Sum256([]byte("plan"))
	left := validAttempt(44, 1, 25)
	left.ContextPlan = &ContextPlanEvidence{ID: 91, SHA256: planHash}
	right := cloneAttempt(left)
	right.ContextPlan = &ContextPlanEvidence{ID: 92, SHA256: planHash}

	if sameAttemptEvidence(left, right) {
		t.Fatal("different Context Plan IDs compared equal")
	}
}

func TestPreparedAssemblySealIncludesContextPlan(t *testing.T) {
	fingerprint := sha256.Sum256([]byte("input"))
	call := PreparedCall{
		RequestSHA256: sha256.Sum256([]byte(`{"model":"gpt-test"}`)),
		ContextPlan:   &ContextPlanEvidence{ID: 91, SHA256: sha256.Sum256([]byte("plan-a"))},
		Quote:         validQuote(44),
	}
	left := preparedAssemblySeal(call, 44, fingerprint)
	call.ContextPlan = &ContextPlanEvidence{ID: 91, SHA256: sha256.Sum256([]byte("plan-b"))}

	if right := preparedAssemblySeal(call, 44, fingerprint); left == right {
		t.Fatal("assembly seal ignored Context Plan hash")
	}
}

type testReserve struct {
	calls       int
	activeCalls int
	targets     []int64
	err         error
	activeErr   error
	facts       *LockedBillingFacts
}

type testReserveFailures struct {
	trigger FinalizationTrigger
	calls   int
}

func (r *testReserveFailures) RecordReserveFailure(_ context.Context, _ Transaction, _ int64, trigger FinalizationTrigger) error {
	r.calls++
	r.trigger = trigger
	return nil
}

type testTx struct{}

func (testTx) BillingTx() {}

type testTransactionRunner struct{}

func (testTransactionRunner) WithinTransaction(_ context.Context, fn func(Transaction) error) error {
	return fn(testTx{})
}

type testOwner struct{ err error }

func (o testOwner) EnsureRunnable(context.Context, Transaction, int64) error { return o.err }

type testQuoteValidator struct{ err error }

func (v testQuoteValidator) ValidateQuote(_ context.Context, run RunSnapshot, preparedRequestSHA256 [32]byte, quote QuoteEvidence) error {
	if v.err != nil {
		return v.err
	}
	if quote.RequestFingerprint != run.RequestFingerprint {
		return errors.New("quote fingerprint differs from locked run")
	}
	if preparedRequestSHA256 == ([32]byte{}) || quote.PreparedRequestSHA256 != preparedRequestSHA256 {
		return errors.New("quote request hash differs from prepared request")
	}
	return nil
}

type trackingQuoteValidator struct {
	calls int
	run   RunSnapshot
	hash  [32]byte
	quote QuoteEvidence
	err   error
}

func (v *trackingQuoteValidator) ValidateQuote(_ context.Context, run RunSnapshot, preparedRequestSHA256 [32]byte, quote QuoteEvidence) error {
	v.calls++
	v.run = run
	v.hash = preparedRequestSHA256
	v.quote = quote
	return v.err
}

type testRunStore struct {
	holdTarget     int64
	zeroHoldTarget bool
}

type orderedRunStore struct {
	testRunStore
	order *[]string
}

func (s orderedRunStore) LockRunAndCharge(ctx context.Context, tx Transaction, runID int64) (LockedRunCharge, error) {
	*s.order = append(*s.order, "run")
	return s.testRunStore.LockRunAndCharge(ctx, tx, runID)
}

type testPriorUsagePricer struct {
	units           int64
	err             error
	calls           int
	beforeAttemptNo uint32
}

func (p *testPriorUsagePricer) PricePriorSucceededUsage(_ context.Context, _ Transaction, _ RunSnapshot, beforeAttemptNo uint32) (int64, error) {
	p.calls++
	p.beforeAttemptNo = beforeAttemptNo
	return p.units, p.err
}

func (testRunStore) LoadRun(_ context.Context, runID int64) (RunSnapshot, error) {
	return RunSnapshot{RunID: runID, UserID: 7, RequestID: "r2", RequestFingerprint: [32]byte{2}}, nil
}
func (s testRunStore) LockRunAndCharge(_ context.Context, _ Transaction, runID int64) (LockedRunCharge, error) {
	target := s.holdTarget
	if target == 0 && !s.zeroHoldTarget {
		target = 25
	}
	return LockedRunCharge{Run: RunSnapshot{RunID: runID}, HoldTargetUnits: target}, nil
}

type persistedFingerprintRunStore struct{ fingerprint [32]byte }

func (s persistedFingerprintRunStore) LoadRun(_ context.Context, runID int64) (RunSnapshot, error) {
	return RunSnapshot{RunID: runID, UserID: 7, RequestID: "r1", RequestFingerprint: s.fingerprint}, nil
}
func (s persistedFingerprintRunStore) LockRunAndCharge(context.Context, Transaction, int64) (LockedRunCharge, error) {
	return LockedRunCharge{}, nil
}

type testAttemptStore struct {
	attempt       ProviderAttempt
	state         string
	terminal      DispatchResult
	recordErr     error
	preparedReads int
	markCalls     int
}

type orderedAttemptStore struct {
	*testAttemptStore
	order *[]string
}

func (s *orderedAttemptStore) GetDispatchedForUpdate(ctx context.Context, tx Transaction, runID int64, attemptNo uint32) (ProviderAttempt, error) {
	*s.order = append(*s.order, "attempt")
	return s.testAttemptStore.GetDispatchedForUpdate(ctx, tx, runID, attemptNo)
}

type testProvider struct {
	calls          int
	preflightCalls int
	preflightErr   error
	proofItems     []billing.UsageItem
	proofHash      [32]byte
	proofErr       error
	capabilities   infraai.CapabilityMetadata
}

type capturingProvider struct {
	testProvider
	attempt ProviderAttempt
}

func (p *capturingProvider) Dispatch(ctx context.Context, attempt ProviderAttempt) (DispatchResult, error) {
	p.attempt = cloneAttempt(attempt)
	return p.testProvider.Dispatch(ctx, attempt)
}

func (p *testProvider) Dispatch(context.Context, ProviderAttempt) (DispatchResult, error) {
	p.calls++
	return DispatchResult{ProviderRequestID: "provider-request-1", ResponseSHA256: sha256.Sum256([]byte("provider-response")), DispatchState: "dispatched", TerminalState: "succeeded", Usage: completeUsageForGatewayTest()}, nil
}

func (p *testProvider) PreflightPrepared(context.Context, ProviderAttempt) error {
	p.preflightCalls++
	return p.preflightErr
}

func (p *testProvider) Capabilities() infraai.CapabilityMetadata {
	if p.capabilities.SafeInputUpperBoundStrategy != "" || p.capabilities.SupportsIdempotencyHeader || len(p.capabilities.SupportedUsageIdentities) != 0 || p.capabilities.SupportsCancelTask {
		return p.capabilities
	}
	return infraai.CapabilityMetadata{SupportsIdempotencyHeader: true, SupportedUsageIdentities: []infraai.UsageIdentity{{Category: infraai.UsageCategoryInput, Unit: "token"}}, SafeInputUpperBoundStrategy: "test_prepared_usage_items_v1"}
}

func (p *testProvider) ProvePreparedUpperBound(_ context.Context, attempt ProviderAttempt) (PreparedUpperBoundProof, error) {
	if p.proofErr != nil {
		return PreparedUpperBoundProof{}, p.proofErr
	}
	items := p.proofItems
	if items == nil {
		items = attempt.Quote.UpperBoundItems
	}
	hash := p.proofHash
	if hash == ([32]byte{}) {
		hash = attempt.RequestSHA256
	}
	return PreparedUpperBoundProof{RequestSHA256: hash, Strategy: p.Capabilities().SafeInputUpperBoundStrategy, Items: append([]billing.UsageItem(nil), items...)}, nil
}

func TestGatewayPreflightFailureNeverMarksOrDispatchesAttempt(t *testing.T) {
	store := &testAttemptStore{attempt: validAttempt(72, 1, 5), state: "prepared"}
	provider := &testProvider{preflightErr: errors.New("etag changed")}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider
	_, err := New(deps).Dispatch(context.Background(), store.attempt)
	if err == nil {
		t.Fatal("preflight failure was accepted")
	}
	if provider.preflightCalls != 1 || store.markCalls != 0 || provider.calls != 0 {
		t.Fatalf("preflight=%d mark=%d dispatch=%d", provider.preflightCalls, store.markCalls, provider.calls)
	}
}

func TestGatewayDispatchPassesPersistedFileManifestUnchanged(t *testing.T) {
	manifest, err := infraai.MarshalPreparedChatFileManifest(infraai.PreparedChatFileManifest{
		Schema: infraai.PreparedChatSchemaFileManifestV1, FileInputMode: "chat_completions",
		Request: json.RawMessage(`{"model":"gpt-5.6","messages":[{"role":"user","content":[{"type":"file_ref","ref":"file-1"}]}]}`),
		Files: []infraai.PreparedFileRef{{
			Ref: "file-1", ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"v1"`,
			Size: 4, MIMEType: "application/pdf", Filename: "report.pdf",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := validAttempt(71, 1, 5)
	attempt.PreparedRequest = append([]byte(nil), manifest...)
	attempt.RequestSHA256 = sha256.Sum256(manifest)
	attempt.Quote.PreparedRequestSHA256 = attempt.RequestSHA256
	attempt.Quote.PreparedRequestSchema = infraai.PreparedChatSchemaFileManifestV1
	store := &testAttemptStore{attempt: cloneAttempt(attempt), state: "prepared"}
	provider := &capturingProvider{}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider

	if _, err := New(deps).Dispatch(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(provider.attempt.PreparedRequest, manifest) || provider.attempt.IdempotencyKey != attempt.IdempotencyKey {
		t.Fatalf("provider attempt changed: %+v", provider.attempt)
	}
}

func completeUsageForGatewayTest() infraai.UsageSnapshot {
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusComplete, []byte(`{"output_tokens":1}`), []infraai.UsageItem{{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1}})
	if err != nil {
		panic(err)
	}
	return usage
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
	s.markCalls++
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
	return Dependencies{Transactions: testTransactionRunner{}, Runs: testRunStore{}, PriorUsage: &testPriorUsagePricer{}, Reserve: reserve, Failures: &testReserveFailures{}, Attempts: attempts, Owner: testOwner{}, Quotes: testQuoteValidator{}}
}

func (r *testReserve) ReserveOrTopUp(_ context.Context, _ Transaction, runID int64, target int64) (LockedBillingFacts, error) {
	r.calls++
	r.targets = append(r.targets, target)
	if r.facts != nil {
		return *r.facts, r.err
	}
	return lockedFacts(runID, target), r.err
}

func TestGatewayReserveTargetsPriorBillableUsagePlusCurrentUpperBound(t *testing.T) {
	reserve := &testReserve{}
	store := &testAttemptStore{}
	deps := testGatewayDependencies(reserve, store)
	deps.Runs = testRunStore{holdTarget: 10}
	priorUsage := &testPriorUsagePricer{units: 8}
	deps.PriorUsage = priorUsage
	fingerprint := [32]byte{}
	call := sealTestCall(50, fingerprint, PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(10)})

	attempt, err := New(deps).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 50, AttemptNo: 2, NewCall: &call})
	if err != nil {
		t.Fatal(err)
	}
	if len(reserve.targets) != 1 || reserve.targets[0] != 18 {
		t.Fatalf("reserve targets=%v, want [18]", reserve.targets)
	}
	if priorUsage.calls != 1 || priorUsage.beforeAttemptNo != 2 {
		t.Fatalf("prior usage calls=%d before_attempt_no=%d", priorUsage.calls, priorUsage.beforeAttemptNo)
	}
	if attempt.Quote.CurrentCallMaxUnits != 10 || attempt.Quote.PriorBillableUnits != 8 || attempt.Quote.TargetHoldUnits != 18 {
		t.Fatalf("persisted cumulative quote=%+v", attempt.Quote)
	}
}

func cumulativeAttempt(runID int64, attemptNo uint32, currentCallMax, priorBillable int64) ProviderAttempt {
	attempt := validAttempt(runID, attemptNo, currentCallMax)
	attempt.Quote.PriorBillableUnits = priorBillable
	attempt.Quote.TargetHoldUnits = priorBillable + currentCallMax
	return attempt
}

func TestGatewayRecoversPersistedCumulativeQuoteWithoutDoubleAddingPriorUsage(t *testing.T) {
	attempt := cumulativeAttempt(55, 2, 10, 8)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Runs = testRunStore{holdTarget: 18}
	deps.PriorUsage = &testPriorUsagePricer{units: 8}

	recovered, err := New(deps).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 55, AttemptNo: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !sameAttemptEvidence(recovered, attempt) {
		t.Fatalf("recovered=%+v, want %+v", recovered, attempt)
	}
}

func TestGatewayRecoveryRejectsPreparedAttemptWithoutCumulativeHold(t *testing.T) {
	store := &testAttemptStore{attempt: validAttempt(51, 2, 10), state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Runs = testRunStore{holdTarget: 10}
	deps.PriorUsage = &testPriorUsagePricer{units: 8}

	if _, err := New(deps).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 51, AttemptNo: 2}); err == nil {
		t.Fatal("underfunded cumulative prepared attempt was recovered")
	}
}

func TestGatewayMarkDispatchedRejectsHoldThatOmitsPriorBillableUsage(t *testing.T) {
	attempt := validAttempt(52, 2, 10)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Runs = testRunStore{holdTarget: 10}
	deps.PriorUsage = &testPriorUsagePricer{units: 8}

	if err := New(deps).MarkDispatched(context.Background(), attempt); err == nil {
		t.Fatal("underfunded cumulative attempt was marked dispatched")
	}
	if store.state != "prepared" {
		t.Fatalf("underfunded attempt state=%q", store.state)
	}
}

func TestGatewayMarkDispatchedRejectsPersistedPriorUsageThatDiffersFromLockedReprice(t *testing.T) {
	attempt := cumulativeAttempt(56, 2, 10, 7)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Runs = testRunStore{holdTarget: 18}
	deps.PriorUsage = &testPriorUsagePricer{units: 8}

	if err := New(deps).MarkDispatched(context.Background(), attempt); err == nil {
		t.Fatal("stale persisted prior usage was marked dispatched")
	}
	if store.state != "prepared" {
		t.Fatalf("stale prior usage advanced state=%q", store.state)
	}
}

func TestGatewayReserveRejectsCumulativeHoldOverflow(t *testing.T) {
	reserve := &testReserve{}
	deps := testGatewayDependencies(reserve, &testAttemptStore{})
	deps.Runs = testRunStore{holdTarget: math.MaxInt64}
	deps.PriorUsage = &testPriorUsagePricer{units: math.MaxInt64}
	call := sealTestCall(53, [32]byte{}, PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(1)})

	if _, err := New(deps).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 53, AttemptNo: 2, NewCall: &call}); err == nil {
		t.Fatal("overflowing cumulative hold target was accepted")
	}
	if reserve.calls != 0 {
		t.Fatalf("overflowing target reached reserve: calls=%d", reserve.calls)
	}
}

func TestGatewayReserveStopsBeforeWalletWhenPriorUsageIsNotPriceable(t *testing.T) {
	reserve := &testReserve{}
	store := &testAttemptStore{}
	deps := testGatewayDependencies(reserve, store)
	deps.PriorUsage = &testPriorUsagePricer{err: ErrUsageIncomplete}
	call := sealTestCall(54, [32]byte{}, PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(10)})

	if _, err := New(deps).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 54, AttemptNo: 2, NewCall: &call}); !errors.Is(err, ErrUsageIncomplete) {
		t.Fatalf("err=%v, want ErrUsageIncomplete", err)
	}
	if reserve.calls != 0 || store.state != "" {
		t.Fatalf("unpriceable prior usage reached reserve/attempt: reserve_calls=%d state=%q", reserve.calls, store.state)
	}
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
	identity := requestIdentity("one")
	fingerprint := requestFingerprint(identity)
	assembler := &testAssembler{}
	gateway := New(Dependencies{Assembler: assembler, Quotes: testQuoteValidator{}, Runs: immutableRunStore{snapshot: RunSnapshot{RunID: 1, UserID: 7, RequestID: "r1", RequestFingerprint: fingerprint}}})
	request := RunRequest{RunID: 1, UserID: 7, RequestID: "r1", Identity: identity}
	if _, err := gateway.AssembleAndQuote(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Identity.NormalizedText = "two"
	var got *Error
	if _, err := gateway.AssembleAndQuote(context.Background(), request); !errors.As(err, &got) || got.Code != ErrCodeFingerprintConflict {
		t.Fatalf("err=%v, want fingerprint conflict", err)
	}
	if assembler.calls != 1 {
		t.Fatalf("assembler calls=%d, want 1", assembler.calls)
	}
}

func TestGatewayAssembleAndQuoteCopiesContextPlanEvidence(t *testing.T) {
	identity := requestIdentity("context plan")
	fingerprint := requestFingerprint(identity)
	plan := &ContextPlanEvidence{ID: 91, SHA256: sha256.Sum256([]byte("ready plan"))}
	gateway := New(Dependencies{
		Assembler: &testAssembler{},
		Quotes:    testQuoteValidator{},
		Runs:      immutableRunStore{snapshot: RunSnapshot{RunID: 41, UserID: 7, RequestID: "r41", RequestFingerprint: fingerprint}},
	})

	call, err := gateway.AssembleAndQuote(context.Background(), RunRequest{
		RunID: 41, UserID: 7, RequestID: "r41", Identity: identity, ContextPlan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.ContextPlan == nil || call.ContextPlan.ID != plan.ID || call.ContextPlan.SHA256 != plan.SHA256 {
		t.Fatalf("prepared call=%+v", call)
	}
	plan.ID = 92
	if call.ContextPlan.ID != 91 {
		t.Fatal("prepared call retained the mutable request pointer")
	}
}

func TestGatewayRejectsFingerprintNotDerivedFromActualInput(t *testing.T) {
	identity := requestIdentity("actual")
	wrong := requestFingerprint(requestIdentity("other"))
	assembler := &testAssembler{}
	gateway := New(Dependencies{Assembler: assembler, Quotes: testQuoteValidator{}, Runs: immutableRunStore{snapshot: RunSnapshot{RunID: 30, UserID: 7, RequestID: "r30", RequestFingerprint: wrong}}})
	_, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 30, UserID: 7, RequestID: "r30", Identity: identity})
	if err == nil || assembler.calls != 0 {
		t.Fatalf("unbound request fingerprint reached assembler: err=%v calls=%d", err, assembler.calls)
	}
}

func TestGatewayRejectsFabricatedIncompletePreparedCall(t *testing.T) {
	store := &testAttemptStore{}
	call := PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(5)}
	_, err := New(testGatewayDependencies(&testReserve{}, store)).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 31, AttemptNo: 1, NewCall: &call})
	if err == nil || store.state != "" {
		t.Fatalf("fabricated prepared call persisted: err=%v state=%q", err, store.state)
	}
}

func TestGatewayRejectsTamperedAssembledCall(t *testing.T) {
	identity := requestIdentity("tamper")
	fingerprint := requestFingerprint(identity)
	runs := immutableRunStore{snapshot: RunSnapshot{RunID: 33, UserID: 7, RequestID: "r33", RequestFingerprint: fingerprint}}
	deps := testGatewayDependencies(&testReserve{}, &testAttemptStore{})
	deps.Assembler = &testAssembler{}
	deps.Runs = runs
	gateway := New(deps)
	call, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 33, UserID: 7, RequestID: "r33", Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	call.Quote.TargetHoldUnits++
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 33, AttemptNo: 1, NewCall: &call}); err == nil {
		t.Fatal("tampered assembled call must be rejected")
	}
}

func TestGatewayRecoveryRejectsInvalidPersistedAttemptEvidence(t *testing.T) {
	body := []byte(`{"model":"x"}`)
	attempt := ProviderAttempt{RunID: 34, AttemptNo: 1, IdempotencyKey: "forged-key", PreparedRequest: body, RequestSHA256: sha256.Sum256(body), Quote: validQuote(5)}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	if _, err := New(testGatewayDependencies(&testReserve{}, store)).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 34, AttemptNo: 1}); err == nil {
		t.Fatal("invalid persisted idempotency key must block recovery")
	}
}

func TestGatewayRecoveryRejectsQuoteBoundToDifferentPreparedRequest(t *testing.T) {
	attempt := validAttempt(48, 1, 5)
	attempt.Quote = quoteWithPreparedRequestHash(attempt.Quote, sha256.Sum256([]byte(`{"model":"other"}`)))
	store := &testAttemptStore{attempt: attempt, state: "prepared"}

	if _, err := New(testGatewayDependencies(&testReserve{}, store)).ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 48, AttemptNo: 1}); err == nil {
		t.Fatal("persisted quote bound to different request bytes was recovered")
	}
}

func TestGatewayReservesBeforeAttemptLock(t *testing.T) {
	call := sealTestCall(35, [32]byte{}, PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(5)})
	gateway := New(testGatewayDependencies(&testReserve{}, &testAttemptStore{}))
	_, _ = gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 35, AttemptNo: 1, NewCall: &call})
	trace := gateway.OperationTrace()
	reserveAt, attemptAt := -1, -1
	for i, step := range trace {
		if step == "reserve_wallet_hold" {
			reserveAt = i
		}
		if step == "lock_attempt" {
			attemptAt = i
		}
	}
	if reserveAt < 0 || attemptAt < 0 || reserveAt > attemptAt {
		t.Fatalf("invalid lock order: %v", trace)
	}
}

func TestGatewayRejectsPersistentFingerprintConflictAfterRestart(t *testing.T) {
	identity := requestIdentity("replay")
	assembler := &testAssembler{}
	gateway := New(Dependencies{Assembler: assembler, Quotes: testQuoteValidator{}, Runs: persistedFingerprintRunStore{fingerprint: [32]byte{1}}})
	_, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 1, UserID: 7, RequestID: "r1", Identity: identity})
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
	identity := requestIdentity("reserve")
	fingerprint := requestFingerprint(identity)
	deps.Runs = immutableRunStore{snapshot: RunSnapshot{RunID: 2, UserID: 7, RequestID: "r2", RequestFingerprint: fingerprint}}
	gateway = New(deps)
	call, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 2, UserID: 7, RequestID: "r2", Identity: identity})
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
	recoveryDeps := testGatewayDependencies(&testReserve{}, recoveryStore)
	recoveryDeps.Runs = immutableRunStore{snapshot: RunSnapshot{RunID: 2, RequestFingerprint: fingerprint}}
	prepared := New(recoveryDeps)
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
		attempt: validAttempt(8, 1, 5),
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

func TestGatewayMarkDispatchedRejectsTamperedQuoteFingerprint(t *testing.T) {
	persisted := validAttempt(39, 1, 5)
	tampered := cloneAttempt(persisted)
	tampered.Quote.RequestFingerprint = [32]byte{1}
	store := &testAttemptStore{attempt: persisted, state: "prepared"}

	if err := New(testGatewayDependencies(&testReserve{}, store)).MarkDispatched(context.Background(), tampered); err == nil {
		t.Fatal("tampered quote fingerprint was accepted")
	}
	if store.state != "prepared" {
		t.Fatalf("tampered quote fingerprint advanced attempt state to %q", store.state)
	}
}

func TestGatewayMarkDispatchedRevalidatesLockedPersistedQuote(t *testing.T) {
	attempt := validAttempt(45, 1, 5)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	validator := &trackingQuoteValidator{}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Quotes = validator

	if err := New(deps).MarkDispatched(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if validator.calls != 1 || validator.run.RunID != attempt.RunID || validator.hash != store.attempt.RequestSHA256 || !equalQuoteEvidence(validator.quote, store.attempt.Quote) {
		t.Fatalf("validator calls=%d run=%+v hash=%x quote=%+v", validator.calls, validator.run, validator.hash, validator.quote)
	}
	if store.markCalls != 1 || store.state != "dispatched" {
		t.Fatalf("mark_calls=%d state=%q", store.markCalls, store.state)
	}
}

func TestGatewayMarkDispatchedRejectsPersistedQuoteBeforeTransition(t *testing.T) {
	attempt := validAttempt(46, 1, 5)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	validator := &trackingQuoteValidator{err: errors.New("persisted quote is invalid")}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Quotes = validator

	if err := New(deps).MarkDispatched(context.Background(), attempt); !errors.Is(err, validator.err) {
		t.Fatalf("err=%v, want validator error", err)
	}
	if validator.calls != 1 || store.markCalls != 0 || store.state != "prepared" {
		t.Fatalf("validator_calls=%d mark_calls=%d state=%q", validator.calls, store.markCalls, store.state)
	}
}

func TestGatewayMarkDispatchedRequiresQuoteValidator(t *testing.T) {
	attempt := validAttempt(47, 1, 5)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Quotes = nil

	if err := New(deps).MarkDispatched(context.Background(), attempt); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err=%v, want ErrNotConfigured", err)
	}
	if store.markCalls != 0 || store.state != "prepared" {
		t.Fatalf("mark_calls=%d state=%q", store.markCalls, store.state)
	}
}

func TestGatewayDoesNotDispatchAnAlreadyDispatchedAttemptAgain(t *testing.T) {
	provider := &testProvider{}
	attempt := validAttempt(9, 1, 5)
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

func TestGatewayRejectsMissingProviderBeforeDispatchedCommit(t *testing.T) {
	attempt := validAttempt(32, 1, 5)
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	_, err := New(testGatewayDependencies(&testReserve{}, store)).Dispatch(context.Background(), attempt)
	if !errors.Is(err, ErrNotConfigured) || store.state != "prepared" {
		t.Fatalf("missing provider committed dispatch: err=%v state=%q", err, store.state)
	}
}

func TestGatewayRejectsProviderProofExceedingQuotedUsageBeforeDispatch(t *testing.T) {
	attempt := validAttempt(36, 1, 5)
	provider := &testProvider{proofItems: []billing.UsageItem{{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 2}}}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider
	if _, err := New(deps).Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("provider proof exceeding the quote was dispatched")
	}
	if provider.calls != 0 || store.state != "prepared" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayAcceptsProviderProofBelowQuotedQuantity(t *testing.T) {
	attempt := validAttempt(44, 1, 5)
	attempt.Quote.UpperBoundItems[0].Quantity = 5
	provider := &testProvider{proofItems: []billing.UsageItem{{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 3}}}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider

	if _, err := New(deps).Dispatch(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || store.state != "succeeded" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayRejectsProviderProofMissingQuotedIdentityBeforeDispatch(t *testing.T) {
	attempt := validAttempt(42, 1, 5)
	attempt.Quote.UpperBoundItems = append(attempt.Quote.UpperBoundItems, billing.UsageItem{
		Category: billing.UsageCategoryOutputText,
		Unit:     "token",
		Quantity: 2,
	})
	provider := &testProvider{
		proofItems: []billing.UsageItem{{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}},
		capabilities: infraai.CapabilityMetadata{
			SupportsIdempotencyHeader: true,
			SupportedUsageIdentities: []infraai.UsageIdentity{
				{Category: infraai.UsageCategoryInput, Unit: "token"},
				{Category: infraai.UsageCategoryOutput, Unit: "token"},
			},
			SafeInputUpperBoundStrategy: "test_prepared_usage_items_v1",
		},
	}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider

	if _, err := New(deps).Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("provider proof missing a quoted usage identity was dispatched")
	}
	if provider.calls != 0 || store.state != "prepared" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayRejectsUsageIdentityMissingFromProviderCapabilityBeforeDispatch(t *testing.T) {
	attempt := validAttempt(43, 1, 5)
	provider := &testProvider{capabilities: infraai.CapabilityMetadata{
		SupportsIdempotencyHeader:   true,
		SupportedUsageIdentities:    []infraai.UsageIdentity{{Category: infraai.UsageCategoryOutput, Unit: "token"}},
		SafeInputUpperBoundStrategy: "test_prepared_usage_items_v1",
	}}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider

	if _, err := New(deps).Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("usage identity missing from provider capability was dispatched")
	}
	if provider.calls != 0 || store.state != "prepared" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayRejectsProviderWithoutSafeUpperBoundStrategyBeforeDispatch(t *testing.T) {
	attempt := validAttempt(40, 1, 5)
	provider := &testProvider{capabilities: infraai.CapabilityMetadata{SupportsIdempotencyHeader: true, SupportedUsageIdentities: []infraai.UsageIdentity{{Category: infraai.UsageCategoryInput, Unit: "token"}}}}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider

	if _, err := New(deps).Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("provider without a safe upper-bound strategy was dispatched")
	}
	if provider.calls != 0 || store.state != "prepared" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayRejectsUpperBoundProofForDifferentPreparedRequest(t *testing.T) {
	attempt := validAttempt(41, 1, 5)
	provider := &testProvider{proofHash: sha256.Sum256([]byte("different-request"))}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider

	if _, err := New(deps).Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("upper-bound proof for a different request was dispatched")
	}
	if provider.calls != 0 || store.state != "prepared" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayAcceptsMediaUsageProofBeforeDispatch(t *testing.T) {
	body := []byte(`{"prompt":"image"}`)
	requestHash := sha256.Sum256(body)
	attempt := ProviderAttempt{
		RunID:           37,
		AttemptNo:       1,
		IdempotencyKey:  attemptKey(37, 1),
		PreparedRequest: body,
		RequestSHA256:   requestHash,
		Quote:           QuoteEvidence{PricingVersion: "v1", PreparedRequestSHA256: requestHash, EffectiveMaxOutputTokens: 1, CurrentCallMaxUnits: 5, TargetHoldUnits: 5, UpperBoundItems: []billing.UsageItem{{Category: billing.UsageCategoryMedia, Unit: "image", Quantity: 1}}},
	}
	provider := &testProvider{
		proofItems: []billing.UsageItem{{Category: billing.UsageCategoryMedia, Unit: "image", Quantity: 1}},
		capabilities: infraai.CapabilityMetadata{
			SupportsIdempotencyHeader:   true,
			SupportedUsageIdentities:    []infraai.UsageIdentity{{Category: infraai.UsageCategoryMedia, Unit: "image"}},
			SafeInputUpperBoundStrategy: "test_prepared_usage_items_v1",
		},
	}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider
	if _, err := New(deps).Dispatch(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || store.state != "succeeded" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayRejectsMissingPreparedUsageProofBeforeDispatch(t *testing.T) {
	attempt := validAttempt(38, 1, 5)
	provider := &testProvider{proofItems: []billing.UsageItem{}}
	store := &testAttemptStore{attempt: attempt, state: "prepared"}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider
	if _, err := New(deps).Dispatch(context.Background(), attempt); err == nil {
		t.Fatal("missing prepared usage proof was dispatched")
	}
	if provider.calls != 0 || store.state != "prepared" {
		t.Fatalf("provider_calls=%d state=%q", provider.calls, store.state)
	}
}

func TestGatewayRequiresActiveHoldBeforeDispatch(t *testing.T) {
	attempt := validAttempt(13, 1, 9)
	reserve := &testReserve{activeErr: errors.New("hold inactive")}
	gateway := New(testGatewayDependencies(reserve, &testAttemptStore{attempt: attempt, state: "prepared"}))
	if err := gateway.MarkDispatched(context.Background(), attempt); err == nil {
		t.Fatal("expected inactive hold to block dispatch")
	}
	if reserve.activeCalls != 1 {
		t.Fatalf("active hold checks=%d, want 1", reserve.activeCalls)
	}
}

func TestGatewayFinalizePassesOnlyRunIdentityToFinalizer(t *testing.T) {
	finalizer := &captureFinalizer{}
	err := New(Dependencies{Finalizer: finalizer}).Finalize(context.Background(), FinalizeRequest{RunID: 14})
	if err != nil {
		t.Fatalf("gateway finalization failed: %v", err)
	}
	if finalizer.calls != 1 || finalizer.request.RunID != 14 {
		t.Fatalf("finalizer was not given run identity: %+v", finalizer)
	}
}

type captureFinalizer struct {
	calls   int
	request FinalizeRequest
}

func (f *captureFinalizer) Finalize(_ context.Context, request FinalizeRequest) error {
	f.calls++
	f.request = request
	return nil
}

func TestGatewayRejectsReplayWithDifferentQuoteEvidence(t *testing.T) {
	gateway := New(testGatewayDependencies(&testReserve{}, &testAttemptStore{}))
	first := sealTestCall(10, [32]byte{}, PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(10)})
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 10, AttemptNo: 1, NewCall: &first}); err != nil {
		t.Fatal(err)
	}
	replay := first
	replay.Quote.EffectiveMaxOutputTokens = 17
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 10, AttemptNo: 1, NewCall: &replay}); err == nil {
		t.Fatal("replay with changed quote evidence must be rejected")
	}
}

func TestGatewayRejectsReplayWithDifferentPreparedRequestUsingOldQuote(t *testing.T) {
	gateway := New(testGatewayDependencies(&testReserve{}, &testAttemptStore{}))
	first := sealTestCall(49, [32]byte{}, PreparedCall{RequestBody: []byte(`{"model":"x"}`), Quote: validQuote(10)})
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 49, AttemptNo: 1, NewCall: &first}); err != nil {
		t.Fatal(err)
	}

	replay := first
	replay.RequestBody = []byte(`{"model":"other"}`)
	replay.RequestSHA256 = sha256.Sum256(replay.RequestBody)
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 49, AttemptNo: 1, NewCall: &replay}); err == nil {
		t.Fatal("replay changed prepared request while reusing old quote")
	}
}

func TestInsufficientBalancePersistsNoProviderAttemptTiming(t *testing.T) {
	reserve := &testReserve{err: errors.New("insufficient balance")}
	gateway := New(testGatewayDependencies(reserve, &testAttemptStore{}))
	body := []byte(`{"model":"gpt-test"}`)
	call := sealTestCall(3, [32]byte{}, PreparedCall{RequestBody: body, Quote: validQuote(10)})
	_, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 3, AttemptNo: 1, NewCall: &call})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != ErrCodeInsufficientBalance {
		t.Fatalf("err=%v, want insufficient balance", err)
	}
	if store := gateway.deps.Attempts.(*testAttemptStore); store.state != "" {
		t.Fatal("insufficient reserve persisted an attempt")
	}
	failures := gateway.deps.Failures.(*testReserveFailures)
	if failures.calls != 1 || failures.trigger != TriggerContinuationTopUpInsufficient {
		t.Fatalf("reserve failure trigger was not persisted: %+v", failures)
	}
}

func TestGatewayInitialInsufficientBalanceCreatesNoAttempt(t *testing.T) {
	reserve := &testReserve{err: errors.New("insufficient balance")}
	store := &testAttemptStore{}
	deps := testGatewayDependencies(reserve, store)
	deps.Runs = testRunStore{zeroHoldTarget: true}
	gateway := New(deps)
	call := sealTestCall(42, [32]byte{}, PreparedCall{RequestBody: []byte(`{"model":"gpt-test"}`), Quote: validQuote(10)})

	_, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 42, AttemptNo: 1, NewCall: &call})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != ErrCodeInsufficientBalance {
		t.Fatalf("err=%v, want insufficient balance", err)
	}
	if store.state != "" {
		t.Fatal("initial insufficient reserve persisted an attempt")
	}
	failures := deps.Failures.(*testReserveFailures)
	if failures.calls != 1 || failures.trigger != TriggerInitialInsufficient {
		t.Fatalf("reserve failure trigger was not persisted: %+v", failures)
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
	attempt := validAttempt(17, 1, 5)
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
	attempt := validAttempt(11, 1, 5)
	deps := testGatewayDependencies(&testReserve{}, &testAttemptStore{attempt: attempt, state: "prepared", recordErr: recordErr})
	deps.Provider = providerError{err: providerErr}
	_, err := New(deps).Dispatch(context.Background(), attempt)
	if !errors.Is(err, providerErr) || !errors.Is(err, recordErr) {
		t.Fatalf("err=%v, want joined provider and persistence errors", err)
	}
}

func TestGatewayRejectsNonTerminalOutcome(t *testing.T) {
	attempt := validAttempt(15, 1, 5)
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

func TestGatewayRejectsSucceededOutcomeWithoutRawUsageEvidence(t *testing.T) {
	attempt := validAttempt(19, 1, 5)
	store := &testAttemptStore{attempt: attempt, state: "dispatched"}
	result := DispatchResult{ProviderRequestID: "provider-request-19", ResponseSHA256: sha256.Sum256([]byte("response")), DispatchState: infraai.DispatchStateDispatched, TerminalState: "succeeded", Usage: infraai.UsageSnapshot{Status: infraai.UsageStatusComplete, Items: []infraai.UsageItem{{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1}}}}
	if err := New(testGatewayDependencies(&testReserve{}, store)).RecordOutcome(context.Background(), attempt, result); err == nil {
		t.Fatal("normalized usage without raw provider evidence must be rejected")
	}
}

func TestGatewayAcceptsCanceledAfterDispatchWithCompleteRawUsage(t *testing.T) {
	attempt := validAttempt(20, 1, 5)
	store := &testAttemptStore{attempt: attempt, state: "dispatched"}
	result := DispatchResult{ProviderRequestID: "provider-request-20", ResponseSHA256: sha256.Sum256([]byte("response")), DispatchState: infraai.DispatchStateDispatched, TerminalState: "canceled", Usage: completeUsageForGatewayTest()}
	if err := New(testGatewayDependencies(&testReserve{}, store)).RecordOutcome(context.Background(), attempt, result); err != nil {
		t.Fatal(err)
	}
	if store.terminal.Usage.Status != infraai.UsageStatusComplete || len(store.terminal.Usage.RawProviderJSON) == 0 {
		t.Fatalf("canceled audit usage was not persisted: %+v", store.terminal)
	}
}

func TestGatewayRejectsInvalidTerminalEvidenceCombinations(t *testing.T) {
	complete := completeUsageForGatewayTest()
	tests := []DispatchResult{
		{DispatchState: infraai.DispatchStateUnknown, TerminalState: "failed", Usage: infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}},
		{DispatchState: infraai.DispatchStateNotDispatched, TerminalState: "failed", Usage: complete},
		{DispatchState: infraai.DispatchStateUnknown, TerminalState: "outcome_unknown", Usage: complete},
		{DispatchState: infraai.DispatchStateDispatched, TerminalState: "canceled", Usage: complete},
	}
	for i, result := range tests {
		if err := validateTerminalOutcome(result); err == nil {
			t.Fatalf("case %d accepted invalid terminal evidence: %+v", i, result)
		}
	}
}

func TestGatewayTerminalOutcomeReplayRequiresIdenticalEvidence(t *testing.T) {
	attempt := validAttempt(18, 1, 5)
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

func TestGatewayTerminalReplayNormalizesPersistedFailureCodeAndIgnoresDiagnosticMetrics(t *testing.T) {
	left := DispatchResult{
		DispatchState: infraai.DispatchStateDispatched, TerminalState: "failed",
		ErrorCode: "ai.provider_failed", Usage: infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable},
	}
	right := left
	right.ErrorCode = ""
	right.FileInputMetrics = &infraai.FileInputMetrics{COSHeadMS: 1, COSStreamMS: 2, MaterializedRequestBytes: 3}
	if !sameTerminalEvidence(left, right) {
		t.Fatal("persisted terminal evidence rejected equivalent runtime diagnostics")
	}
	right.ErrorCode = "ai.provider.file_part_rejected"
	if sameTerminalEvidence(left, right) {
		t.Fatal("different stable application error codes were treated as equivalent")
	}
}

func TestGatewayRecordOutcomeLocksRunBeforeAttempt(t *testing.T) {
	attempt := validAttempt(81, 1, 5)
	baseStore := &testAttemptStore{attempt: attempt, state: "dispatched"}
	order := make([]string, 0, 2)
	deps := testGatewayDependencies(&testReserve{}, baseStore)
	deps.Runs = orderedRunStore{order: &order}
	deps.Attempts = &orderedAttemptStore{testAttemptStore: baseStore, order: &order}
	result := DispatchResult{
		ProviderRequestID: "provider-request-81", ResponseSHA256: sha256.Sum256([]byte("response-81")),
		DispatchState: infraai.DispatchStateDispatched, TerminalState: "succeeded", Usage: completeUsageForGatewayTest(),
	}
	if err := New(deps).RecordOutcome(context.Background(), attempt, result); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "run" || order[1] != "attempt" {
		t.Fatalf("terminal lock order=%v", order)
	}
}

func TestGatewayAssembleValidatesPersistedRunIdentityBeforeAssembler(t *testing.T) {
	identity := requestIdentity("identity")
	fingerprint := requestFingerprint(identity)
	assembler := &testAssembler{}
	store := immutableRunStore{snapshot: RunSnapshot{RunID: 12, UserID: 7, RequestID: "stored-request", RequestFingerprint: fingerprint}}
	gateway := New(Dependencies{Assembler: assembler, Quotes: testQuoteValidator{}, Runs: store})
	_, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 12, UserID: 7, RequestID: "caller-request", Identity: identity})
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
	return LockedRunCharge{Run: s.snapshot, HoldTargetUnits: 25}, nil
}

type providerError struct{ err error }

func (p providerError) Dispatch(context.Context, ProviderAttempt) (DispatchResult, error) {
	return DispatchResult{}, p.err
}

func (p providerError) Capabilities() infraai.CapabilityMetadata {
	return infraai.CapabilityMetadata{SupportsIdempotencyHeader: true, SupportedUsageIdentities: []infraai.UsageIdentity{{Category: infraai.UsageCategoryInput, Unit: "token"}}, SafeInputUpperBoundStrategy: "test_prepared_usage_items_v1"}
}

func (p providerError) ProvePreparedUpperBound(_ context.Context, attempt ProviderAttempt) (PreparedUpperBoundProof, error) {
	return PreparedUpperBoundProof{RequestSHA256: attempt.RequestSHA256, Strategy: p.Capabilities().SafeInputUpperBoundStrategy, Items: append([]billing.UsageItem(nil), attempt.Quote.UpperBoundItems...)}, nil
}

func (p providerError) PreflightPrepared(context.Context, ProviderAttempt) error { return nil }

func TestTerminalProviderRejectionPreservesStableApplicationErrorCode(t *testing.T) {
	providerErr := infraai.NewProviderError(infraai.ProviderOutcomeRejected, "provider-request-file-1", apperror.New(
		"ai.provider.file_part_rejected", apperror.CategoryDependency, 502, apperror.Permanent,
		"ai.provider.file_part_rejected", nil, "上游渠道拒绝文件内容",
	))
	result := terminalResultForProviderError(DispatchResult{}, providerErr)
	if result.ErrorCode != "ai.provider.file_part_rejected" || result.ProviderRequestID != "provider-request-file-1" ||
		result.DispatchState != infraai.DispatchStateDispatched || result.TerminalState != "failed" {
		t.Fatalf("terminal result=%+v", result)
	}
}
