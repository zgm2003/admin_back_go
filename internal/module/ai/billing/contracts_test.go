package billing

import (
	"errors"
	"testing"
)

func TestUsageItemValidationRejectsZeroAndNegativeQuantity(t *testing.T) {
	for _, quantity := range []int64{0, -1} {
		item := UsageItem{Category: UsageCategoryInputText, Unit: "token", Quantity: quantity}
		if err := item.Validate(); !errors.Is(err, ErrInvalidUsageItem) {
			t.Fatalf("quantity=%d error=%v", quantity, err)
		}
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

func TestAttemptTransitionsRejectDispatchWithoutPreparation(t *testing.T) {
	if !CanTransitionAttemptState(AttemptStatePrepared, AttemptStateDispatched) {
		t.Fatal("prepared -> dispatched must be valid")
	}
	if CanTransitionAttemptState(AttemptStateFailed, AttemptStateDispatched) {
		t.Fatal("failed -> dispatched must be invalid")
	}
}
