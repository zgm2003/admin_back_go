package aigateway

import (
	"context"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type finalizerStore struct {
	outcomes  []AttemptOutcome
	facts     SettlementFacts
	released  int
	captured  int
	finalized map[int64]bool
	last      FinalizeInput
}

func (s *finalizerStore) WithLockedSettlement(_ context.Context, _ int64, decide func([]AttemptOutcome, SettlementFacts) (FinalizeInput, error)) (bool, error) {
	input, err := decide(s.outcomes, s.facts)
	if err != nil {
		return false, err
	}
	if s.finalized[input.RunID] {
		return false, nil
	}
	s.last = input
	if input.BillingStatus == billing.BillingStatusSettled && input.ActualUnits > 0 {
		s.captured++
	} else {
		s.released++
	}
	if s.finalized == nil {
		s.finalized = make(map[int64]bool)
	}
	s.finalized[input.RunID] = true
	return true, nil
}

func completeSnapshot() infraai.UsageSnapshot {
	return infraai.UsageSnapshot{Status: infraai.UsageStatusComplete, Items: []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 1}}}
}

func TestRunFinalizerExcludesFailedAttemptAndSettlesOnce(t *testing.T) {
	store := &finalizerStore{outcomes: []AttemptOutcome{
		{State: string(billing.AttemptStateFailed), DispatchState: string(billing.DispatchStateDispatched), Usage: completeSnapshot(), UsageComplete: true},
		{State: string(billing.AttemptStateSucceeded), Usage: completeSnapshot(), UsageComplete: true},
	}, facts: SettlementFacts{ActualUnits: 3, HoldUnits: 5}}
	f := NewFinalizer(store)
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 1, ActualUnits: 3, HoldUnits: 5}); err != nil {
		t.Fatal(err)
	}
	if store.captured != 1 || store.released != 0 || store.last.BillingReason != billing.BillingReasonSettledCompleteUsage {
		t.Fatalf("store=%+v", store)
	}
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 1, ActualUnits: 3, HoldUnits: 5}); err != nil {
		t.Fatal(err)
	}
	if store.captured != 1 {
		t.Fatalf("settlement was not idempotent; captured=%d", store.captured)
	}
}

func TestRunFinalizerReleasesIncompleteAndOverhold(t *testing.T) {
	store := &finalizerStore{outcomes: []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), Usage: infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}}}, facts: SettlementFacts{ActualUnits: 1, HoldUnits: 1}}
	f := NewFinalizer(store)
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 2, ActualUnits: 1, HoldUnits: 1}); err != nil {
		t.Fatal(err)
	}
	if store.released != 1 || store.last.BillingReason != billing.BillingReasonUnbilledUsageIncomplete {
		t.Fatalf("incomplete settlement=%+v released=%d", store.last, store.released)
	}
	store.outcomes = []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), Usage: completeSnapshot(), UsageComplete: true}}
	store.facts = SettlementFacts{ActualUnits: 2, HoldUnits: 1}
	f = NewFinalizer(store)
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 3, ActualUnits: 2, HoldUnits: 1}); err != nil {
		t.Fatal(err)
	}
	if store.last.BillingReason != billing.BillingReasonUnbilledOverHold || store.released != 2 {
		t.Fatalf("overhold settlement=%+v released=%d", store.last, store.released)
	}
}

func TestLedgerSummaryContainsNoRequestMaterial(t *testing.T) {
	summary := LedgerSummary(42, "Model Display", "model-id")
	if summary != "Agent #42 · Model Display" {
		t.Fatalf("summary=%q", summary)
	}
}

func TestRunFinalizerDoesNotCreateZeroValueCapture(t *testing.T) {
	store := &finalizerStore{outcomes: []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), Usage: completeSnapshot(), UsageComplete: true}}, facts: SettlementFacts{ActualUnits: 0, HoldUnits: 1}}
	f := NewFinalizer(store)
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 4, ActualUnits: 0, HoldUnits: 1}); err != nil {
		t.Fatal(err)
	}
	if store.captured != 0 {
		t.Fatalf("zero-value settlement created capture: %d", store.captured)
	}
}

func TestRunFinalizerFailsClosedForUnknownAttemptState(t *testing.T) {
	store := &finalizerStore{outcomes: []AttemptOutcome{{State: "unexpected", Usage: completeSnapshot(), UsageComplete: true}}, facts: SettlementFacts{ActualUnits: 1, HoldUnits: 1}}
	f := NewFinalizer(store)
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 5, ActualUnits: 1, HoldUnits: 1}); err != nil {
		t.Fatal(err)
	}
	if store.released != 1 || store.captured != 0 || store.last.BillingReason != billing.BillingReasonUnbilledUsageIncomplete {
		t.Fatalf("unknown state settled unexpectedly: %+v", store)
	}
}

func TestRunFinalizerUsesPersistedSettlementFactsInsteadOfCallerAmounts(t *testing.T) {
	store := &finalizerStore{
		outcomes: []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), Usage: completeSnapshot(), UsageComplete: true}},
		facts:    SettlementFacts{ActualUnits: 2, HoldUnits: 3},
	}
	if err := NewFinalizer(store).Finalize(context.Background(), FinalizeInput{RunID: 6, ActualUnits: 99, HoldUnits: 99}); err != nil {
		t.Fatal(err)
	}
	if store.captured != 1 || store.last.ActualUnits != 2 || store.last.HoldUnits != 3 {
		t.Fatalf("finalizer trusted caller settlement: %+v", store)
	}
}

func TestRunFinalizerFailsClosedForEmptyAndNonterminalOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		runState string
		outcomes []AttemptOutcome
		reason   billing.BillingReason
	}{
		{name: "empty success", runState: "success", reason: billing.BillingReasonUnbilledUsageIncomplete},
		{name: "empty canceled", runState: "canceled", reason: billing.BillingReasonUnbilledUsageIncomplete},
		{name: "dispatched", runState: "running", outcomes: []AttemptOutcome{{State: string(billing.AttemptStateDispatched), DispatchState: string(billing.DispatchStateDispatched)}}, reason: billing.BillingReasonUnbilledUsageIncomplete},
		{name: "canceled after dispatch", runState: "canceled", outcomes: []AttemptOutcome{{State: string(billing.AttemptStateCanceled), DispatchState: string(billing.DispatchStateDispatched)}}, reason: billing.BillingReasonUnbilledUsageIncomplete},
		{name: "canceled before dispatch", runState: "canceled", outcomes: []AttemptOutcome{{State: string(billing.AttemptStateCanceled), DispatchState: string(billing.DispatchStateNotDispatched)}}, reason: billing.BillingReasonReleasedBeforeDispatch},
		{name: "provider failed after dispatch", runState: "failed", outcomes: []AttemptOutcome{{State: string(billing.AttemptStateFailed), DispatchState: string(billing.DispatchStateDispatched)}}, reason: billing.BillingReasonReleasedProviderFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &finalizerStore{outcomes: tc.outcomes, facts: SettlementFacts{ActualUnits: 1, HoldUnits: 1}}
			if err := NewFinalizer(store).Finalize(context.Background(), FinalizeInput{RunID: 20, RunStatus: tc.runState}); err != nil {
				t.Fatal(err)
			}
			if store.captured != 0 || store.released != 1 || store.last.BillingReason != tc.reason {
				t.Fatalf("unexpected settlement: %+v", store)
			}
		})
	}
}

func TestRunFinalizerDoesNotTrustUsageCompleteFlag(t *testing.T) {
	store := &finalizerStore{
		outcomes: []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), UsageComplete: true, Usage: infraai.UsageSnapshot{Status: infraai.UsageStatusReported}}},
		facts:    SettlementFacts{ActualUnits: 1, HoldUnits: 1},
	}
	if err := NewFinalizer(store).Finalize(context.Background(), FinalizeInput{RunID: 21}); err != nil {
		t.Fatal(err)
	}
	if store.captured != 0 || store.released != 1 || store.last.BillingReason != billing.BillingReasonUnbilledUsageIncomplete {
		t.Fatalf("usage-complete flag bypassed snapshot validation: %+v", store)
	}
}
