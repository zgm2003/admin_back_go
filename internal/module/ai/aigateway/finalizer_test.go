package aigateway

import (
	"context"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type finalizerStore struct {
	outcomes []AttemptOutcome
	released int
	captured int
	last     FinalizeInput
}

func (s *finalizerStore) LoadAttemptOutcomes(context.Context, int64) ([]AttemptOutcome, error) {
	return s.outcomes, nil
}
func (s *finalizerStore) CaptureAndRelease(_ context.Context, input FinalizeInput) error {
	s.captured++
	s.last = input
	return nil
}
func (s *finalizerStore) Release(_ context.Context, input FinalizeInput) error {
	s.released++
	s.last = input
	return nil
}

func completeSnapshot() infraai.UsageSnapshot {
	return infraai.UsageSnapshot{Status: infraai.UsageStatusComplete, Items: []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 1}}}
}

func TestRunFinalizerExcludesFailedAttemptAndSettlesOnce(t *testing.T) {
	store := &finalizerStore{outcomes: []AttemptOutcome{
		{State: string(billing.AttemptStateFailed), Usage: completeSnapshot(), UsageComplete: true},
		{State: string(billing.AttemptStateSucceeded), Usage: completeSnapshot(), UsageComplete: true},
	}}
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
	store := &finalizerStore{outcomes: []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), Usage: infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}}}}
	f := NewFinalizer(store)
	if err := f.Finalize(context.Background(), FinalizeInput{RunID: 2, ActualUnits: 1, HoldUnits: 1}); err != nil {
		t.Fatal(err)
	}
	if store.released != 1 || store.last.BillingReason != billing.BillingReasonUnbilledUsageIncomplete {
		t.Fatalf("incomplete settlement=%+v released=%d", store.last, store.released)
	}
	store.outcomes = []AttemptOutcome{{State: string(billing.AttemptStateSucceeded), Usage: completeSnapshot(), UsageComplete: true}}
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
