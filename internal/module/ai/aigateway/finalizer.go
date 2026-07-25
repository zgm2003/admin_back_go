package aigateway

import (
	"context"
	"fmt"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type AttemptOutcome struct {
	AttemptNo     uint32
	State         string
	DispatchState string
	Usage         infraai.UsageSnapshot
	UsageComplete bool
	ActualUnits   int64
}

// SettlementFacts are derived from persisted Run pricing, usage and Hold facts.
// The finalizer never accepts customer-facing money amounts from its caller.
type SettlementFacts struct {
	ActualUnits int64
	HoldUnits   int64
	Items       []billing.UsageChargeItem
}

type FinalizationStore interface {
	// WithLockedSettlement must use one transaction and acquire locks in this
	// order: Run, Charge, wallet, Hold. It must read attempt outcomes and
	// settlement facts under those locks, invoke decide, then atomically apply
	// the resulting terminal transition and idempotency fence. A settled input
	// with ActualUnits > 0 captures and releases the Hold; every other terminal
	// result releases it without a capture.
	WithLockedSettlement(context.Context, int64, func([]AttemptOutcome, SettlementFacts) (FinalizeInput, error)) (bool, error)
}

type RunFinalizer struct {
	store FinalizationStore
}

func NewFinalizer(store FinalizationStore) *RunFinalizer {
	return &RunFinalizer{store: store}
}

func (f *RunFinalizer) Finalize(ctx context.Context, input FinalizeInput) error {
	if f == nil || f.store == nil {
		return ErrNotConfigured
	}
	_, err := f.store.WithLockedSettlement(ctx, input.RunID, func(outcomes []AttemptOutcome, facts SettlementFacts) (FinalizeInput, error) {
		if facts.ActualUnits < 0 || facts.HoldUnits < 0 {
			return FinalizeInput{}, fmt.Errorf("negative persisted settlement units")
		}
		input.ActualUnits = facts.ActualUnits
		input.HoldUnits = facts.HoldUnits
		input.Items = append([]billing.UsageChargeItem(nil), facts.Items...)
		complete, reason := completeUsage(outcomes)
		if !complete {
			input.BillingStatus = billing.BillingStatusUnbilled
			if reason == billing.BillingReasonReleasedBeforeDispatch || reason == billing.BillingReasonReleasedProviderFailed || reason == billing.BillingReasonReleasedOutcomeUnknown {
				input.BillingStatus = billing.BillingStatusReleased
			}
			input.BillingReason = reason
			return input, nil
		}
		if input.ActualUnits > input.HoldUnits {
			input.BillingStatus = billing.BillingStatusUnbilled
			input.BillingReason = billing.BillingReasonUnbilledOverHold
			return input, nil
		}
		input.BillingStatus = billing.BillingStatusSettled
		input.BillingReason = billing.BillingReasonSettledCompleteUsage
		return input, nil
	})
	return err
}

func completeUsage(outcomes []AttemptOutcome) (bool, billing.BillingReason) {
	if len(outcomes) == 0 {
		return false, billing.BillingReasonUnbilledUsageIncomplete
	}
	hasSucceeded := false
	hasProviderFailure := false
	hasBeforeDispatchFailure := false
	for _, outcome := range outcomes {
		switch strings.TrimSpace(outcome.State) {
		case string(billing.AttemptStateFailed):
			switch billing.DispatchState(strings.TrimSpace(outcome.DispatchState)) {
			case billing.DispatchStateDispatched:
				hasProviderFailure = true
			case billing.DispatchStateNotDispatched:
				hasBeforeDispatchFailure = true
			case billing.DispatchStateUnknown:
				return false, billing.BillingReasonReleasedOutcomeUnknown
			default:
				return false, billing.BillingReasonUnbilledUsageIncomplete
			}
		case string(billing.AttemptStateSucceeded):
			hasSucceeded = true
			if !outcome.Usage.Complete() {
				return false, billing.BillingReasonUnbilledUsageIncomplete
			}
		case string(billing.AttemptStateDispatched):
			return false, billing.BillingReasonUnbilledUsageIncomplete
		case string(billing.AttemptStateCanceled):
			switch billing.DispatchState(strings.TrimSpace(outcome.DispatchState)) {
			case billing.DispatchStateNotDispatched:
				hasBeforeDispatchFailure = true
			case billing.DispatchStateUnknown:
				return false, billing.BillingReasonReleasedOutcomeUnknown
			default:
				return false, billing.BillingReasonUnbilledUsageIncomplete
			}
		case string(billing.AttemptStateOutcomeUnknown):
			return false, billing.BillingReasonReleasedOutcomeUnknown
		default:
			return false, billing.BillingReasonUnbilledUsageIncomplete
		}
	}
	if hasSucceeded {
		return true, billing.BillingReasonSettledCompleteUsage
	}
	if hasProviderFailure {
		return false, billing.BillingReasonReleasedProviderFailed
	}
	if hasBeforeDispatchFailure {
		return false, billing.BillingReasonReleasedBeforeDispatch
	}
	return false, billing.BillingReasonUnbilledUsageIncomplete
}

func LedgerSummary(agentID int64, displayName, modelID string) string {
	model := strings.TrimSpace(displayName)
	if model == "" {
		model = strings.TrimSpace(modelID)
	}
	if model == "" {
		model = "unknown"
	}
	return fmt.Sprintf("Agent #%d · %s", agentID, model)
}
