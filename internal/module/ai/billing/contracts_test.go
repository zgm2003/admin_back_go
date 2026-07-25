package billing

import (
	"errors"
	"testing"
)

func TestUsageItemValidationAcceptsZeroAndRejectsNegativeQuantity(t *testing.T) {
	for _, quantity := range []int64{0, 1} {
		item := UsageItem{Category: UsageCategoryInputText, Unit: "token", Quantity: quantity}
		if err := item.Validate(); err != nil {
			t.Fatalf("quantity=%d error=%v", quantity, err)
		}
	}
	item := UsageItem{Category: UsageCategoryInputText, Unit: "token", Quantity: -1}
	if err := item.Validate(); !errors.Is(err, ErrInvalidUsageItem) {
		t.Fatalf("negative quantity error=%v", err)
	}
}

func TestUsageItemNormalizesTier(t *testing.T) {
	item := UsageItem{Category: UsageCategoryOutputText, Unit: " token ", TierKey: "  standard  ", Quantity: 1}
	normalized, err := item.Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Unit != "token" || normalized.TierKey != "standard" {
		t.Fatalf("normalized=%+v", normalized)
	}
}

func TestBillingStatusTransitionsRejectInvalidMoves(t *testing.T) {
	if !CanTransitionBillingStatus(BillingStatusPending, BillingStatusHeld) {
		t.Fatal("pending -> held must be valid")
	}
	if CanTransitionBillingStatus(BillingStatusSettled, BillingStatusHeld) {
		t.Fatal("settled -> held must be invalid")
	}
	if CanTransitionHoldStatus(HoldStatusCaptured, HoldStatusActive) {
		t.Fatal("captured -> active must be invalid")
	}
}

func TestStateTransitionsRejectUnknownStateSelfTransitions(t *testing.T) {
	if CanTransitionBillingStatus(BillingStatus("unknown"), BillingStatus("unknown")) {
		t.Fatal("unknown billing state must not become idempotently valid")
	}
	if CanTransitionHoldStatus(HoldStatus("unknown"), HoldStatus("unknown")) {
		t.Fatal("unknown hold state must not become idempotently valid")
	}
	if CanTransitionAttemptState(AttemptState("unknown"), AttemptState("unknown")) {
		t.Fatal("unknown attempt state must not become idempotently valid")
	}
}

func TestAttemptTransitionsRejectDispatchWithoutPreparation(t *testing.T) {
	if !CanTransitionAttemptState(AttemptStatePrepared, AttemptStateDispatched) {
		t.Fatal("prepared -> dispatched must be valid")
	}
	if CanTransitionAttemptState(AttemptStateFailed, AttemptStateDispatched) {
		t.Fatal("failed -> dispatched must be invalid")
	}
}

func TestOutcomeEvidenceCarriesNonSecretProviderFacts(t *testing.T) {
	wantHash := SHA256Digest{1, 2, 3}
	evidence := OutcomeEvidence{
		AttemptID:         9,
		DispatchState:     DispatchStateDispatched,
		State:             AttemptStateSucceeded,
		ProviderRequestID: "provider-request-7",
		ResponseSHA256:    wantHash,
	}
	if evidence.DispatchState != DispatchStateDispatched || evidence.ProviderRequestID != "provider-request-7" || evidence.ResponseSHA256 != wantHash {
		t.Fatalf("outcome evidence=%+v", evidence)
	}
}
