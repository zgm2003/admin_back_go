package aigateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type finalizerStore struct {
	facts    FinalizationFacts
	terminal bool
	calls    int
	last     SettlementDecision
	applyErr error
}

type noOpFinalizerStore struct{ facts FinalizationFacts }

func (s noOpFinalizerStore) WithLockedSettlement(_ context.Context, _ int64, decide func(FinalizationFacts) (SettlementDecision, error)) (FinalizationApplyResult, error) {
	if _, err := decide(s.facts); err != nil {
		return FinalizationApplyResult{}, err
	}
	return FinalizationApplyResult{}, nil
}

func (s *finalizerStore) WithLockedSettlement(_ context.Context, runID int64, decide func(FinalizationFacts) (SettlementDecision, error)) (FinalizationApplyResult, error) {
	s.calls++
	if s.terminal {
		return FinalizationApplyResult{Replayed: true}, nil
	}
	if s.facts.Run.RunID != runID {
		return FinalizationApplyResult{}, errors.New("wrong locked run")
	}
	decision, err := decide(s.facts)
	if err != nil {
		return FinalizationApplyResult{}, err
	}
	if s.applyErr != nil {
		return FinalizationApplyResult{}, s.applyErr
	}
	s.last = decision
	s.terminal = true
	return FinalizationApplyResult{Applied: true}, nil
}

type finalizerPricer struct {
	quote SettlementQuote
	err   error
	calls int
	input SettlementPricingInput
}

func (p *finalizerPricer) PriceSettlement(_ context.Context, input SettlementPricingInput) (SettlementQuote, error) {
	p.calls++
	p.input = input
	return p.quote, p.err
}

func usableUsage() infraai.UsageSnapshot {
	return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
}

func succeededAttempt(id int64, no uint32) FinalizationAttempt {
	return FinalizationAttempt{ID: id, AttemptNo: no, EvidenceKind: AttemptEvidencePaid, State: billing.AttemptStateSucceeded, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-request", ResponseSHA256: sha256.Sum256([]byte("provider-response")), Usage: completeUsageForGatewayTest()}
}

func stoppedAttempt(id int64, no uint32) FinalizationAttempt {
	return FinalizationAttempt{ID: id, AttemptNo: no, EvidenceKind: AttemptEvidencePaid, State: billing.AttemptStateCanceled, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-request", ResponseSHA256: sha256.Sum256([]byte("provider-response")), Usage: completeUsageForGatewayTest()}
}

func pricedItem(attemptID int64, amount int64) billing.UsageChargeItem {
	return billing.UsageChargeItem{AttemptID: attemptID, Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1, UnitPriceUnits: amount, UnitScale: 1, AmountUnits: amount}
}

func finalizationFacts(trigger FinalizationTrigger, attempts ...FinalizationAttempt) FinalizationFacts {
	for index := range attempts {
		attempts[index].RunID = 44
		if attempts[index].EvidenceKind == "" {
			attempts[index].EvidenceKind = AttemptEvidencePaid
		}
	}
	currentAttemptID := int64(0)
	candidate := FinalizationCandidate{}
	if len(attempts) > 0 {
		currentAttemptID = attempts[len(attempts)-1].ID
		candidate = FinalizationCandidate{AttemptID: currentAttemptID, JSON: `{"answer":"candidate"}`}
	}
	facts := FinalizationFacts{
		Run:              RunSnapshot{RunID: 44, UserID: 9, PricingSnapshotJSON: `{"catalog":"persisted"}`, BillingStatus: billing.BillingStatusHeld, BillingReason: billing.BillingReasonHeld, AgentID: 7, ModelID: "model-7", ModelDisplayName: "Model Seven"},
		Charge:           FinalizationCharge{ID: 101, RunID: 44, UserID: 9, HeldUnits: 5, HeldAuditMax: 5, Status: billing.ChargeStatusOpen},
		Hold:             FinalizationHold{ID: 102, WalletID: 103, RunID: 44, UserID: 9, HeldUnits: 5, HeldAuditMax: 5, Status: billing.HoldStatusActive},
		Trigger:          trigger,
		Attempts:         attempts,
		CurrentAttemptID: currentAttemptID,
		Candidate:        candidate,
	}
	if trigger == TriggerInitialInsufficient {
		facts.Run.BillingStatus = billing.BillingStatusPending
		facts.Run.BillingReason = billing.BillingReasonPending
		facts.Charge.HeldUnits = 0
		facts.Charge.HeldAuditMax = 0
		facts.Hold = FinalizationHold{}
	}
	return facts
}

func initialInsufficientFacts() FinalizationFacts {
	facts := finalizationFacts(TriggerInitialInsufficient)
	facts.Run.BillingStatus = billing.BillingStatusPending
	facts.Run.BillingReason = billing.BillingReasonPending
	facts.Charge.HeldUnits = 0
	facts.Charge.HeldAuditMax = 0
	facts.Hold = FinalizationHold{}
	return facts
}

func TestRunFinalizerDecisionMatrix(t *testing.T) {
	tests := []struct {
		name       string
		trigger    FinalizationTrigger
		attempts   []FinalizationAttempt
		quote      SettlementQuote
		pricerErr  error
		runStatus  string
		billing    billing.BillingStatus
		reason     billing.BillingReason
		money      SettlementMoneyAction
		candidate  SettlementCandidateAction
		priceCalls int
	}{
		{"success", TriggerSuccess, []FinalizationAttempt{succeededAttempt(1, 1)}, SettlementQuote{ActualUnits: 3, Items: []billing.UsageChargeItem{pricedItem(1, 3)}}, nil, "success", billing.BillingStatusSettled, billing.BillingReasonSettledCompleteUsage, SettlementMoneyCapture, SettlementCandidatePublish, 1},
		{"success incomplete", TriggerSuccess, []FinalizationAttempt{succeededAttempt(1, 1)}, SettlementQuote{}, ErrUsageIncomplete, "failed", billing.BillingStatusUnbilled, billing.BillingReasonUnbilledUsageIncomplete, SettlementMoneyRelease, SettlementCandidateDiscard, 1},
		{"user stop before dispatch", TriggerUserStopBeforeDispatch, []FinalizationAttempt{{ID: 1, AttemptNo: 1, State: billing.AttemptStateCanceled, DispatchState: billing.DispatchStateNotDispatched, Usage: usableUsage()}}, SettlementQuote{}, nil, "canceled", billing.BillingStatusReleased, billing.BillingReasonReleasedBeforeDispatch, SettlementMoneyRelease, SettlementCandidateDiscard, 0},
		{"user stop complete", TriggerUserStop, []FinalizationAttempt{succeededAttempt(1, 1), succeededAttempt(2, 2)}, SettlementQuote{ActualUnits: 3, Items: []billing.UsageChargeItem{pricedItem(1, 1), pricedItem(2, 2)}}, nil, "canceled", billing.BillingStatusSettled, billing.BillingReasonSettledCompleteUsage, SettlementMoneyCapture, SettlementCandidateDiscard, 1},
		{"user stop incomplete", TriggerUserStop, []FinalizationAttempt{succeededAttempt(1, 1)}, SettlementQuote{}, ErrUsageIncomplete, "canceled", billing.BillingStatusUnbilled, billing.BillingReasonUnbilledUsageIncomplete, SettlementMoneyRelease, SettlementCandidateDiscard, 1},
		{"provider failed", TriggerProviderFailed, []FinalizationAttempt{{ID: 1, AttemptNo: 1, State: billing.AttemptStateFailed, DispatchState: billing.DispatchStateDispatched, Usage: usableUsage()}}, SettlementQuote{}, nil, "failed", billing.BillingStatusReleased, billing.BillingReasonReleasedProviderFailed, SettlementMoneyRelease, SettlementCandidateDiscard, 0},
		{"unknown", TriggerOutcomeUnknown, []FinalizationAttempt{{ID: 1, AttemptNo: 1, State: billing.AttemptStateOutcomeUnknown, DispatchState: billing.DispatchStateUnknown, Usage: usableUsage()}}, SettlementQuote{}, nil, "outcome_unknown", billing.BillingStatusReleased, billing.BillingReasonReleasedOutcomeUnknown, SettlementMoneyRelease, SettlementCandidateDiscard, 0},
		{"initial insufficient", TriggerInitialInsufficient, nil, SettlementQuote{}, nil, "failed", billing.BillingStatusReleased, billing.BillingReasonReleasedInsufficientBalance, SettlementMoneyRelease, SettlementCandidateDiscard, 0},
		{"continuation insufficient no prior success", TriggerContinuationTopUpInsufficient, nil, SettlementQuote{}, nil, "failed", billing.BillingStatusReleased, billing.BillingReasonReleasedInsufficientBalance, SettlementMoneyRelease, SettlementCandidateDiscard, 0},
		{"continuation insufficient complete", TriggerContinuationTopUpInsufficient, []FinalizationAttempt{succeededAttempt(1, 1)}, SettlementQuote{ActualUnits: 2, Items: []billing.UsageChargeItem{pricedItem(1, 2)}}, nil, "failed", billing.BillingStatusSettled, billing.BillingReasonSettledCompleteUsage, SettlementMoneyCapture, SettlementCandidateDiscard, 1},
		{"continuation insufficient incomplete", TriggerContinuationTopUpInsufficient, []FinalizationAttempt{succeededAttempt(1, 1)}, SettlementQuote{}, ErrUsageIncomplete, "failed", billing.BillingStatusUnbilled, billing.BillingReasonUnbilledUsageIncomplete, SettlementMoneyRelease, SettlementCandidateDiscard, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &finalizerStore{facts: finalizationFacts(tc.trigger, tc.attempts...)}
			pricer := &finalizerPricer{quote: tc.quote, err: tc.pricerErr}
			if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
				t.Fatal(err)
			}
			got := store.last
			if got.RunStatus != tc.runStatus || got.BillingStatus != tc.billing || got.BillingReason != tc.reason || got.MoneyAction != tc.money || got.CandidateAction != tc.candidate || pricer.calls != tc.priceCalls {
				t.Fatalf("decision=%+v pricer_calls=%d", got, pricer.calls)
			}
		})
	}
}

func TestRunFinalizerOverHoldDiscardsCandidateAndRecordsAnomaly(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 6, Items: []billing.UsageChargeItem{pricedItem(1, 6)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.RunStatus != "failed" || store.last.BillingStatus != billing.BillingStatusUnbilled || store.last.BillingReason != billing.BillingReasonUnbilledOverHold || store.last.MoneyAction != SettlementMoneyRelease || store.last.CandidateAction != SettlementCandidateDiscard || store.last.BillingAnomaly == "" {
		t.Fatalf("overhold decision=%+v", store.last)
	}
}

func TestRunFinalizerExcludesFailedUsageAndPricesWholePersistedRunOnce(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerContinuationTopUpInsufficient, succeededAttempt(1, 1), succeededAttempt(2, 2), FinalizationAttempt{ID: 3, AttemptNo: 3, State: billing.AttemptStateFailed, DispatchState: billing.DispatchStateDispatched, Usage: usableUsage()})}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 3, Items: []billing.UsageChargeItem{pricedItem(1, 1), pricedItem(2, 2)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if pricer.calls != 1 || pricer.input.Run.PricingSnapshotJSON != `{"catalog":"persisted"}` || len(pricer.input.Attempts) != 2 || pricer.input.Attempts[0].ID == 3 || pricer.input.Attempts[1].ID == 3 {
		t.Fatalf("unexpected pricing input: %+v", pricer.input)
	}
}

func TestRunFinalizerStopsAfterDispatchWithoutCompleteClosedAttempts(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerUserStop, succeededAttempt(1, 1), FinalizationAttempt{ID: 2, AttemptNo: 2, State: billing.AttemptStateCanceled, DispatchState: billing.DispatchStateDispatched, Usage: usableUsage()})}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.RunStatus != "canceled" || store.last.BillingStatus != billing.BillingStatusUnbilled || store.last.CandidateAction != SettlementCandidateDiscard || pricer.calls != 0 {
		t.Fatalf("incomplete stopped run was priced: decision=%+v calls=%d", store.last, pricer.calls)
	}
}

func TestRunFinalizerRejectsInvalidPricerOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		quote SettlementQuote
	}{
		{"item sum", SettlementQuote{ActualUnits: 3, Items: []billing.UsageChargeItem{pricedItem(1, 2)}}},
		{"duplicate", SettlementQuote{ActualUnits: 2, Items: []billing.UsageChargeItem{pricedItem(1, 1), pricedItem(1, 1)}}},
		{"foreign attempt", SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(99, 1)}}},
		{"negative actual", SettlementQuote{ActualUnits: -1}},
		{"invalid item field", SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{{AttemptID: 1, Category: billing.UsageCategoryInputText, Quantity: 1, UnitPriceUnits: 1, UnitScale: 1, AmountUnits: 1}}}},
		{"amount overflow", SettlementQuote{ActualUnits: math.MaxInt64, Items: []billing.UsageChargeItem{pricedItem(1, math.MaxInt64), {AttemptID: 1, Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 1, UnitPriceUnits: 1, UnitScale: 1, AmountUnits: 1}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &finalizerStore{facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))}
			err := NewFinalizer(store, &finalizerPricer{quote: tc.quote}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
			if err == nil || store.terminal {
				t.Fatalf("invalid quote finalized: err=%v decision=%+v", err, store.last)
			}
		})
	}
}

func TestRunFinalizerRejectsInconsistentLockedHoldAudit(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))}
	store.facts.Hold.HeldAuditMax = 4
	err := NewFinalizer(store, &finalizerPricer{}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil || store.terminal {
		t.Fatalf("inconsistent hold audit was finalized: err=%v", err)
	}
}

func TestRunFinalizerRejectsInvalidLockedFactGraph(t *testing.T) {
	tests := []struct {
		name   string
		facts  FinalizationFacts
		mutate func(*FinalizationFacts)
	}{
		{name: "charge belongs to another run", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Charge.RunID++ }},
		{name: "hold belongs to another run", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Hold.RunID++ }},
		{name: "hold belongs to another user", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Hold.UserID++ }},
		{name: "run billing status is already terminal", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Run.BillingStatus = billing.BillingStatusSettled }},
		{name: "run billing reason is not held", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Run.BillingReason = billing.BillingReasonPending }},
		{name: "charge is already terminal", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Charge.Status = billing.ChargeStatusSettled }},
		{name: "hold is already terminal", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Hold.Status = billing.HoldStatusCaptured }},
		{name: "duplicate attempt id", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1), succeededAttempt(2, 2)), mutate: func(f *FinalizationFacts) { f.Attempts[1].ID = f.Attempts[0].ID }},
		{name: "duplicate attempt number", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1), succeededAttempt(2, 2)), mutate: func(f *FinalizationFacts) { f.Attempts[1].AttemptNo = f.Attempts[0].AttemptNo }},
		{name: "succeeded attempt was not dispatched", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Attempts[0].DispatchState = billing.DispatchStateNotDispatched }},
		{name: "failed attempt has unknown dispatch", facts: finalizationFacts(TriggerProviderFailed, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateFailed, DispatchState: billing.DispatchStateDispatched, Usage: usableUsage()}), mutate: func(f *FinalizationFacts) { f.Attempts[0].DispatchState = billing.DispatchStateUnknown }},
		{name: "outcome unknown attempt was dispatched", facts: finalizationFacts(TriggerOutcomeUnknown, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateOutcomeUnknown, DispatchState: billing.DispatchStateUnknown, Usage: usableUsage()}), mutate: func(f *FinalizationFacts) { f.Attempts[0].DispatchState = billing.DispatchStateDispatched }},
		{name: "current attempt is missing", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.CurrentAttemptID = 99 }},
		{name: "candidate attempt is missing", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Candidate.AttemptID = 99 }},
		{name: "candidate does not match current attempt", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1), succeededAttempt(2, 2)), mutate: func(f *FinalizationFacts) { f.Candidate.AttemptID = 1 }},
		{name: "stopped attempt is missing", facts: finalizationFacts(TriggerUserStop, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.StoppedAttemptID = 99 }},
		{name: "stopped attempt is not dispatched canceled current attempt", facts: finalizationFacts(TriggerUserStop, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.StoppedAttemptID = 1 }},
		{name: "stopped attempt is set for success", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.StoppedAttemptID = 1 }},
		{name: "initial insufficient contains an attempt", facts: initialInsufficientFacts(), mutate: func(f *FinalizationFacts) { f.Attempts = []FinalizationAttempt{succeededAttempt(1, 1)} }},
		{name: "initial insufficient run is held", facts: initialInsufficientFacts(), mutate: func(f *FinalizationFacts) {
			f.Run.BillingStatus = billing.BillingStatusHeld
			f.Run.BillingReason = billing.BillingReasonHeld
		}},
		{name: "non-initial finalization has no hold", facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1)), mutate: func(f *FinalizationFacts) { f.Hold = FinalizationHold{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(&tc.facts)
			store := &finalizerStore{facts: tc.facts}
			err := NewFinalizer(store, &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
			if err == nil || store.terminal {
				t.Fatalf("invalid fact graph finalized: err=%v decision=%+v", err, store.last)
			}
		})
	}
}

func TestRunFinalizerRejectsChargeOwnedByAnotherUser(t *testing.T) {
	facts := finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))
	facts.Charge.UserID = facts.Run.UserID + 1
	store := &finalizerStore{facts: facts}

	err := NewFinalizer(store, &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil || store.terminal {
		t.Fatalf("cross-user charge finalized: err=%v decision=%+v", err, store.last)
	}
}

func TestRunFinalizerRejectsAttemptOwnedByAnotherRun(t *testing.T) {
	facts := finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))
	facts.Attempts[0].RunID = facts.Run.RunID + 1
	store := &finalizerStore{facts: facts}

	err := NewFinalizer(store, &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil || store.terminal {
		t.Fatalf("cross-run attempt finalized: err=%v decision=%+v", err, store.last)
	}
}

func TestRunFinalizerRejectsProviderFailedTriggerForNonFailedCurrentAttempt(t *testing.T) {
	facts := finalizationFacts(TriggerProviderFailed, succeededAttempt(1, 1))
	store := &finalizerStore{facts: facts}

	err := NewFinalizer(store, &finalizerPricer{}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil || store.terminal {
		t.Fatalf("provider_failed accepted non-failed current attempt: err=%v decision=%+v", err, store.last)
	}
}

func TestRunFinalizerRejectsOutcomeUnknownTriggerForKnownCurrentAttempt(t *testing.T) {
	facts := finalizationFacts(TriggerOutcomeUnknown, succeededAttempt(1, 1))
	store := &finalizerStore{facts: facts}

	err := NewFinalizer(store, &finalizerPricer{}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil || store.terminal {
		t.Fatalf("outcome_unknown accepted known current attempt: err=%v decision=%+v", err, store.last)
	}
}

func TestRunFinalizerDoesNotBillOrPublishLegacyAttempt(t *testing.T) {
	facts := finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))
	facts.Attempts[0].EvidenceKind = AttemptEvidenceLegacyUnbillable
	store := &finalizerStore{facts: facts}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}

	err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if pricer.calls != 0 {
		t.Fatalf("legacy attempt reached pricer: calls=%d", pricer.calls)
	}
	if store.last.BillingStatus != billing.BillingStatusUnbilled || store.last.MoneyAction != SettlementMoneyRelease || store.last.CandidateAction != SettlementCandidateDiscard {
		t.Fatalf("legacy attempt decision = %+v", store.last)
	}
	if store.last.RunStatus != "failed" {
		t.Fatalf("legacy attempt run status = %q", store.last.RunStatus)
	}
}

func TestRunFinalizerRejectsForgedPersistedTerminalEvidence(t *testing.T) {
	tests := []struct {
		name   string
		facts  FinalizationFacts
		mutate func(*FinalizationFacts)
	}{
		{
			name:  "complete succeeded result has no response hash",
			facts: finalizationFacts(TriggerSuccess, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateSucceeded, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-1", Usage: completeUsageForGatewayTest()}),
			mutate: func(*FinalizationFacts) {
			},
		},
		{
			name:  "complete succeeded result has no provider id",
			facts: finalizationFacts(TriggerSuccess, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateSucceeded, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-1", Usage: completeUsageForGatewayTest()}),
			mutate: func(f *FinalizationFacts) {
				f.Attempts[0].ResponseSHA256 = sha256.Sum256([]byte("response"))
				f.Attempts[0].ProviderRequestID = ""
			},
		},
		{
			name:  "complete canceled result has no provider id",
			facts: finalizationFacts(TriggerUserStop, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateCanceled, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-1", Usage: completeUsageForGatewayTest()}),
			mutate: func(f *FinalizationFacts) {
				f.Attempts[0].ResponseSHA256 = sha256.Sum256([]byte("response"))
				f.Attempts[0].ProviderRequestID = ""
			},
		},
		{
			name:  "complete usage raw hash is forged",
			facts: finalizationFacts(TriggerSuccess, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateSucceeded, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-1", Usage: completeUsageForGatewayTest()}),
			mutate: func(f *FinalizationFacts) {
				f.Attempts[0].ResponseSHA256 = sha256.Sum256([]byte("response"))
				f.Attempts[0].Usage.ResponseSHA256 = sha256.Sum256([]byte("forged"))
			},
		},
		{
			name:  "not dispatched failed result has response evidence",
			facts: finalizationFacts(TriggerProviderFailed, FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateFailed, DispatchState: billing.DispatchStateNotDispatched, Usage: usableUsage()}),
			mutate: func(f *FinalizationFacts) {
				f.Attempts[0].ResponseSHA256 = sha256.Sum256([]byte("response"))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(&tc.facts)
			store := &finalizerStore{facts: tc.facts}
			err := NewFinalizer(store, &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
			if err == nil || store.terminal {
				t.Fatalf("forged terminal evidence finalized: err=%v decision=%+v", err, store.last)
			}
		})
	}
}

func TestRunFinalizerRejectsPublishCandidateOutsideBillableAttempts(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))}
	store.facts.Candidate.AttemptID = 99
	err := NewFinalizer(store, &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil || store.terminal {
		t.Fatalf("foreign candidate was published: err=%v decision=%+v", err, store.last)
	}
}

func TestRunFinalizerCapturesZeroAndReplaysWithoutRepricing(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 0}}
	f := NewFinalizer(store, pricer)
	if err := f.Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.MoneyAction != SettlementMoneyCapture || store.last.ActualUnits != 0 {
		t.Fatalf("zero settlement must retain capture action: %+v", store.last)
	}
	if err := f.Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if pricer.calls != 1 || store.calls != 2 {
		t.Fatalf("replay repriced or did not use idempotency fence: calls=%d store=%d", pricer.calls, store.calls)
	}
}

func TestRunFinalizerRejectsIntermediateFacts(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts("", FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateDispatched, DispatchState: billing.DispatchStateDispatched, Usage: usableUsage()})}
	err := NewFinalizer(store, &finalizerPricer{}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if !errors.Is(err, ErrFinalizationPending) || store.terminal {
		t.Fatalf("intermediate facts were terminalized: err=%v", err)
	}
}

func TestRunFinalizerRejectsStoreNoOpWithoutReplayFence(t *testing.T) {
	facts := finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))
	err := NewFinalizer(noOpFinalizerStore{facts: facts}, &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(1, 1)}}}).Finalize(context.Background(), FinalizeRequest{RunID: 44})
	if err == nil {
		t.Fatal("store no-op without applied or replayed fence must fail closed")
	}
}

func TestRunFinalizerBillsCompleteStoppedAttemptAndPriorSuccess(t *testing.T) {
	stopped := stoppedAttempt(2, 2)
	facts := finalizationFacts(TriggerUserStop, succeededAttempt(1, 1), stopped)
	facts.StoppedAttemptID = 2
	store := &finalizerStore{facts: facts}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 2, Items: []billing.UsageChargeItem{pricedItem(1, 1), pricedItem(2, 1)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.BillingStatus != billing.BillingStatusSettled || len(pricer.input.Attempts) != 2 {
		t.Fatalf("complete stopped attempt was not billed: decision=%+v pricing=%+v", store.last, pricer.input)
	}
}

func TestRunFinalizerSuccessIgnoresPriorFailedAttemptForCompleteness(t *testing.T) {
	failed := FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateFailed, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-request", ResponseSHA256: sha256.Sum256([]byte("provider-response")), Usage: completeUsageForGatewayTest()}
	succeeded := succeededAttempt(2, 2)
	facts := finalizationFacts(TriggerSuccess, failed, succeeded)
	facts.Candidate.AttemptID = 2
	store := &finalizerStore{facts: facts}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 1, Items: []billing.UsageChargeItem{pricedItem(2, 1)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.BillingStatus != billing.BillingStatusSettled || len(pricer.input.Attempts) != 1 || pricer.input.Attempts[0].ID != 2 {
		t.Fatalf("failed audit blocked success: decision=%+v pricing=%+v", store.last, pricer.input)
	}
}

func TestRunFinalizerUserStopIgnoresFailedAuditAndIncludesStoppedAttempt(t *testing.T) {
	failed := FinalizationAttempt{ID: 1, AttemptNo: 1, State: billing.AttemptStateFailed, DispatchState: billing.DispatchStateDispatched, ProviderRequestID: "provider-request", ResponseSHA256: sha256.Sum256([]byte("provider-response")), Usage: completeUsageForGatewayTest()}
	succeeded := succeededAttempt(2, 2)
	stopped := stoppedAttempt(3, 3)
	facts := finalizationFacts(TriggerUserStop, failed, succeeded, stopped)
	facts.StoppedAttemptID = 3
	store := &finalizerStore{facts: facts}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 2, Items: []billing.UsageChargeItem{pricedItem(2, 1), pricedItem(3, 1)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.BillingStatus != billing.BillingStatusSettled || len(pricer.input.Attempts) != 2 {
		t.Fatalf("failed audit blocked stopped settlement: decision=%+v pricing=%+v", store.last, pricer.input)
	}
}

func TestRunFinalizerAllowsInitialInsufficientWithoutHold(t *testing.T) {
	facts := initialInsufficientFacts()
	store := &finalizerStore{facts: facts}
	if err := NewFinalizer(store, &finalizerPricer{}).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.BillingReason != billing.BillingReasonReleasedInsufficientBalance {
		t.Fatalf("initial insufficient decision=%+v", store.last)
	}
}

func TestRunFinalizerUsesOnlyPersistedFactsAndLedgerIdentity(t *testing.T) {
	store := &finalizerStore{facts: finalizationFacts(TriggerSuccess, succeededAttempt(1, 1))}
	pricer := &finalizerPricer{quote: SettlementQuote{ActualUnits: 4, Items: []billing.UsageChargeItem{pricedItem(1, 4)}}}
	if err := NewFinalizer(store, pricer).Finalize(context.Background(), FinalizeRequest{RunID: 44}); err != nil {
		t.Fatal(err)
	}
	if store.last.ActualUnits != 4 || store.last.HoldUnits != 5 || store.last.LedgerSummary != "Agent #7 · Model Seven" {
		t.Fatalf("decision did not use locked facts: %+v", store.last)
	}
}
