package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
)

type fakeTextGateway struct {
	calls          []string
	assembled      aigateway.PreparedCall
	reserved       aigateway.ProviderAttempt
	reserveInput   aigateway.ReserveAndPrepareInput
	dispatchInput  aigateway.ProviderAttempt
	assembleErr    error
	reserveErr     error
	dispatchErr    error
	dispatchResult aigateway.DispatchResult
}

func TestPricingSnapshotHoldUsesMostExpensiveFrozenTier(t *testing.T) {
	rates := []pricing.Rate{
		{Category: pricing.InputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 1, UnitScale: 1},
		{Category: pricing.InputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 2, UnitScale: 1},
		{Category: pricing.OutputTokens, Unit: "token", TierKey: "short_context", PriceUnits: 3, UnitScale: 1},
		{Category: pricing.OutputTokens, Unit: "token", TierKey: "long_context", PriceUnits: 6, UnitScale: 1},
	}
	model := officialmodel.ResolvedModel{
		Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: "gpt-tiered", ContextWindowTokens: 200, MaxOutputTokens: 100, ContextTierThresholdTokens: 50},
		EffectivePrice: pricing.PriceBook{ModelID: "gpt-tiered", ContextTierThresholdTokens: 50, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
		PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: "openai", RequestedModelID: model.Model.ModelID, EffectiveMaxOutputTokens: 10, MultiplierPPM: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := aigateway.ParsePricingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := quotePricingSnapshot(snapshot, []billing.UsageItem{
		{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 10},
		{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 10},
	}, "hold")
	if err != nil || quote.AmountUnits != 80 {
		t.Fatalf("upper-bound quote = %#v, %v", quote, err)
	}
}

func TestPricingSnapshotHoldCoversClaudeCacheWriteUpperBound(t *testing.T) {
	rates := []pricing.Rate{
		{Category: pricing.InputTokens, Unit: "token", PriceUnits: 3, UnitScale: 1},
		{Category: pricing.CacheRead, Unit: "token", PriceUnits: 1, UnitScale: 1},
		{Category: pricing.CacheWrite, Unit: "token", TierKey: "5m", PriceUnits: 4, UnitScale: 1},
		{Category: pricing.CacheWrite, Unit: "token", TierKey: "1h", PriceUnits: 6, UnitScale: 1},
		{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 8, UnitScale: 1},
	}
	model := officialmodel.ResolvedModel{
		Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "anthropic", ModelID: "claude-tiered", ContextWindowTokens: 200, MaxOutputTokens: 100},
		EffectivePrice: pricing.PriceBook{ModelID: "claude-tiered", Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
		PriceSourceURL: "https://anthropic.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: "anthropic", RequestedModelID: model.Model.ModelID, EffectiveMaxOutputTokens: 10, MultiplierPPM: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := aigateway.ParsePricingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := quotePricingSnapshot(snapshot, []billing.UsageItem{
		{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 10},
		{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 10},
	}, "hold")
	if err != nil || quote.AmountUnits != 140 {
		t.Fatalf("cache-safe upper-bound quote = %#v, %v", quote, err)
	}
}

type recordingTextChatFactory struct {
	calls  int
	input  aichat.EngineConfig
	engine infraai.Engine
}

func (f *recordingTextChatFactory) NewEngine(_ context.Context, input aichat.EngineConfig) (infraai.Engine, error) {
	f.calls++
	f.input = input
	return f.engine, nil
}

type recordingTextToolFactory struct {
	calls  int
	input  aitool.EngineConfig
	engine infraai.Engine
}

func (f *recordingTextToolFactory) NewEngine(_ context.Context, input aitool.EngineConfig) (infraai.Engine, error) {
	f.calls++
	f.input = input
	return f.engine, nil
}

func (f *fakeTextGateway) AssembleAndQuote(_ context.Context, _ aigateway.RunRequest) (aigateway.PreparedCall, error) {
	f.calls = append(f.calls, "assemble")
	return f.assembled, f.assembleErr
}

func (f *fakeTextGateway) ReserveAndPrepare(_ context.Context, input aigateway.ReserveAndPrepareInput) (aigateway.ProviderAttempt, error) {
	f.calls = append(f.calls, "reserve")
	f.reserveInput = input
	return f.reserved, f.reserveErr
}

func (f *fakeTextGateway) Dispatch(_ context.Context, attempt aigateway.ProviderAttempt) (aigateway.DispatchResult, error) {
	f.calls = append(f.calls, "dispatch")
	f.dispatchInput = attempt
	return f.dispatchResult, f.dispatchErr
}

func TestRunTextGatewayAttemptUsesQuoteReserveDispatchOrder(t *testing.T) {
	body := []byte(`{"model":"gpt-4.1","messages":[]}`)
	call := aigateway.PreparedCall{RequestBody: append([]byte(nil), body...)}
	attempt := aigateway.ProviderAttempt{RunID: 51, AttemptNo: 1, IdempotencyKey: "run:51:attempt:1", PreparedRequest: append([]byte(nil), body...)}
	gateway := &fakeTextGateway{assembled: call, reserved: attempt}

	_, err := runTextGatewayAttempt(context.Background(), gateway, aigateway.RunRequest{RunID: 51, UserID: 7, RequestID: "request-1"}, 1, false)

	if err != nil {
		t.Fatalf("runTextGatewayAttempt: %v", err)
	}
	if !reflect.DeepEqual(gateway.calls, []string{"assemble", "reserve", "dispatch"}) {
		t.Fatalf("calls = %#v", gateway.calls)
	}
	if gateway.reserveInput.NewCall == nil || !bytes.Equal(gateway.reserveInput.NewCall.RequestBody, body) {
		t.Fatalf("reserve input = %#v", gateway.reserveInput)
	}
	if !bytes.Equal(gateway.dispatchInput.PreparedRequest, body) || gateway.dispatchInput.IdempotencyKey != attempt.IdempotencyKey {
		t.Fatalf("dispatch input = %#v", gateway.dispatchInput)
	}
}

func TestPaidTextExecutorRoutesEngineFactoryByTaskKind(t *testing.T) {
	chatEngine := infraai.NewFakeEngine("chat")
	toolEngine := infraai.NewFakeEngine("tool")
	chatFactory := &recordingTextChatFactory{engine: chatEngine}
	toolFactory := &recordingTextToolFactory{engine: toolEngine}
	executor := &paidTextTaskExecutor{chatEngine: chatFactory, toolEngine: toolFactory}
	config := aichat.EngineConfig{EngineType: infraai.EngineTypeOpenAI, BaseURL: "https://provider.test/v1", APIKey: "secret"}

	got, err := executor.newTextEngine(context.Background(), aitext.KindToolDraft, config)
	if err != nil || got != toolEngine {
		t.Fatalf("tool engine=%T error=%v", got, err)
	}
	if chatFactory.calls != 0 || toolFactory.calls != 1 {
		t.Fatalf("tool route calls: chat=%d tool=%d", chatFactory.calls, toolFactory.calls)
	}
	if toolFactory.input.EngineType != config.EngineType || toolFactory.input.BaseURL != config.BaseURL || toolFactory.input.APIKey != config.APIKey {
		t.Fatalf("tool config=%#v", toolFactory.input)
	}

	got, err = executor.newTextEngine(context.Background(), aitext.KindText, config)
	if err != nil || got != chatEngine {
		t.Fatalf("chat engine=%T error=%v", got, err)
	}
	if chatFactory.calls != 1 || toolFactory.calls != 1 {
		t.Fatalf("chat route calls: chat=%d tool=%d", chatFactory.calls, toolFactory.calls)
	}
}

func TestToolDraftCandidateEncoderRejectsInvalidBusinessCandidate(t *testing.T) {
	result := &infraai.ChatResult{Answer: `{"ok":true,"draft":{"code":"BAD CODE"}}`}

	candidate, err := encodeTextResultCandidate(aitext.KindToolDraft, result)

	if err != nil {
		t.Fatalf("business rejection must remain a known dispatched outcome: %v", err)
	}
	if candidate != nil {
		t.Fatalf("invalid tool draft became publishable candidate: %q", *candidate)
	}
}

func TestRunTextGatewayAttemptPreparedRecoveryIsByteIdentical(t *testing.T) {
	prepared := []byte("{\n  \"messages\": [{\"role\":\"user\",\"content\":\"exact bytes\"}]\n}")
	attempt := aigateway.ProviderAttempt{RunID: 51, AttemptNo: 1, IdempotencyKey: "run:51:attempt:1", PreparedRequest: append([]byte(nil), prepared...)}
	gateway := &fakeTextGateway{reserved: attempt}

	_, err := runTextGatewayAttempt(context.Background(), gateway, aigateway.RunRequest{RunID: 51, UserID: 7, RequestID: "request-1"}, 1, true)

	if err != nil {
		t.Fatalf("runTextGatewayAttempt: %v", err)
	}
	if !reflect.DeepEqual(gateway.calls, []string{"reserve", "dispatch"}) {
		t.Fatalf("recovery calls = %#v", gateway.calls)
	}
	if gateway.reserveInput.NewCall != nil {
		t.Fatalf("prepared recovery rebuilt call: %#v", gateway.reserveInput.NewCall)
	}
	if !bytes.Equal(gateway.dispatchInput.PreparedRequest, prepared) {
		t.Fatalf("prepared bytes changed: got=%q want=%q", gateway.dispatchInput.PreparedRequest, prepared)
	}
}

func TestRunTextGatewayAttemptReserveFailureDoesNotDispatch(t *testing.T) {
	gateway := &fakeTextGateway{
		assembled:  aigateway.PreparedCall{RequestBody: []byte(`{"model":"gpt-4.1"}`)},
		reserveErr: &aigateway.Error{Code: aigateway.ErrCodeInsufficientBalance, Status: 409, Message: "low balance"},
	}

	_, err := runTextGatewayAttempt(context.Background(), gateway, aigateway.RunRequest{RunID: 51, UserID: 7, RequestID: "request-1"}, 1, false)

	var gatewayErr *aigateway.Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != aigateway.ErrCodeInsufficientBalance {
		t.Fatalf("error = %v", err)
	}
	if !reflect.DeepEqual(gateway.calls, []string{"assemble", "reserve"}) {
		t.Fatalf("calls = %#v", gateway.calls)
	}
}

func TestRunTextGatewayAttemptTreatsLostDispatchCASAsAnotherWorkerOwnership(t *testing.T) {
	gateway := &fakeTextGateway{
		reserved:    aigateway.ProviderAttempt{RunID: 51, AttemptNo: 1, PreparedRequest: []byte(`{"model":"gpt-4.1"}`)},
		dispatchErr: &aigateway.Error{Code: aigateway.ErrCodePreparedMissing, Status: 409, Message: "already dispatched"},
	}

	_, err := runTextGatewayAttempt(context.Background(), gateway, aigateway.RunRequest{RunID: 51, UserID: 7, RequestID: "request-1"}, 1, true)
	if !errors.Is(err, ErrTextAttemptOwnedElsewhere) {
		t.Fatalf("error=%v, want ownership fence", err)
	}
}

func TestGatewayRunSnapshotRejectsTerminalRunBeforeReserve(t *testing.T) {
	_, err := gatewayRunSnapshot(airun.Run{
		ID: 51, UserID: 7, RequestID: "request-1", RequestFingerprint: bytes.Repeat([]byte{1}, sha256.Size),
		Status: "timeout", BillingStatus: string(billing.BillingStatusPending),
	})
	if err == nil {
		t.Fatal("terminal run remained eligible for paid dispatch")
	}
}

func TestTextPreDispatchErrorCodeIsStableByFailureClass(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "price", err: pricing.ErrPriceUnavailable, want: aitext.ErrorCodePriceUnavailable},
		{name: "upper bound", err: &aigateway.Error{Code: aigateway.ErrCodeInvalidPrepared, Message: "provider lacks required safe upper-bound usage capability", Status: 409}, want: aitext.ErrorCodeUnsafeUpperBound},
		{name: "configuration", err: errors.New("engine unavailable"), want: aitext.ErrorCodeConfiguration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := textPreDispatchErrorCode(tc.err); got != tc.want {
				t.Fatalf("code = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTextDispatchedAttemptWaitsForGenerationDeadlineBeforeOutcomeUnknown(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	dispatchedAt := now.Add(-aitext.GenerateTimeout + time.Second)
	attempt := replycommand.Attempt{State: replycommand.AttemptDispatched, DispatchedAt: &dispatchedAt}

	if textDispatchedAttemptStale(attempt, now) {
		t.Fatal("active dispatched attempt was classified outcome_unknown")
	}
	dispatchedAt = now.Add(-aitext.GenerateTimeout)
	if !textDispatchedAttemptStale(attempt, now) {
		t.Fatal("expired dispatched attempt remained active")
	}
}

func TestDeriveTextFinalizationTriggerUsesLocalFailureAfterCandidatePersistenceError(t *testing.T) {
	task := aitext.TextTask{LastErrorCode: aitext.ErrorCodeConfiguration}
	attempts := []replycommand.Attempt{{State: replycommand.AttemptSucceeded, DispatchState: infraai.DispatchStateDispatched}}

	trigger, err := deriveTextFinalizationTrigger(task, attempts)
	if err != nil {
		t.Fatalf("derive trigger: %v", err)
	}
	if trigger != aigateway.TriggerLocalFailure {
		t.Fatalf("trigger=%q, want %q", trigger, aigateway.TriggerLocalFailure)
	}
}

type textFinalizationCaptureStore struct {
	facts    aigateway.FinalizationFacts
	decision aigateway.SettlementDecision
}

func (s *textFinalizationCaptureStore) WithLockedSettlement(_ context.Context, _ int64, decide func(aigateway.FinalizationFacts) (aigateway.SettlementDecision, error)) (aigateway.FinalizationApplyResult, error) {
	decision, err := decide(s.facts)
	if err != nil {
		return aigateway.FinalizationApplyResult{}, err
	}
	s.decision = decision
	return aigateway.FinalizationApplyResult{Applied: true}, nil
}

func TestTextMissingUsageReleasesHoldAsUnbilledAndDiscardsDraft(t *testing.T) {
	fingerprint := sha256.Sum256([]byte("request-1"))
	responseHash := sha256.Sum256([]byte("provider response"))
	candidate := `{"version":"ai_text_result_v1","answer":"draft that must not publish"}`
	store := &textFinalizationCaptureStore{facts: aigateway.FinalizationFacts{
		Run: aigateway.RunSnapshot{
			RunID: 51, UserID: 7, RequestID: "request-1", RequestFingerprint: fingerprint,
			PricingSnapshotJSON: testPricingSnapshotJSON(), BillingStatus: billing.BillingStatusHeld,
			BillingReason: billing.BillingReasonHeld, AgentID: 5, ModelID: "model-7",
		},
		Charge: aigateway.FinalizationCharge{ID: 61, RunID: 51, UserID: 7, HeldUnits: 10, HeldAuditMax: 10, Status: billing.ChargeStatusOpen},
		Hold: aigateway.FinalizationHold{
			ID: 71, WalletID: 81, RunID: 51, UserID: 7, HeldUnits: 10, HeldAuditMax: 10, Status: billing.HoldStatusActive,
		},
		Attempts: []aigateway.FinalizationAttempt{{
			ID: 91, RunID: 51, AttemptNo: 1, EvidenceKind: aigateway.AttemptEvidencePaid,
			State: billing.AttemptStateSucceeded, DispatchState: billing.DispatchStateDispatched,
			Usage:             infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable},
			ProviderRequestID: "provider-request-1", ResponseSHA256: responseHash,
		}},
		Trigger: aigateway.TriggerSuccess, CurrentAttemptID: 91,
		Candidate: aigateway.FinalizationCandidate{AttemptID: 91, JSON: candidate},
	}}

	err := aigateway.NewFinalizer(store, persistedSettlementPricer{}).Finalize(context.Background(), aigateway.FinalizeRequest{RunID: 51})
	if err != nil {
		t.Fatalf("finalize missing usage: %v", err)
	}
	decision := store.decision
	if decision.RunStatus != "failed" || decision.BillingStatus != billing.BillingStatusUnbilled ||
		decision.BillingReason != billing.BillingReasonUnbilledUsageIncomplete || decision.MoneyAction != aigateway.SettlementMoneyRelease ||
		decision.CandidateAction != aigateway.SettlementCandidateDiscard || decision.ActualUnits != 0 {
		t.Fatalf("missing usage decision=%+v", decision)
	}
}
