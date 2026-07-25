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
	LoadAttemptOutcomes(context.Context, int64) ([]AttemptOutcome, error)
	LoadSettlementFacts(context.Context, int64) (SettlementFacts, error)
	// CaptureAndRelease and Release must atomically enforce terminal Run/Charge
	// idempotency and return whether this call applied a new transition.
	CaptureAndRelease(context.Context, FinalizeInput) (bool, error)
	Release(context.Context, FinalizeInput) (bool, error)
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
	outcomes, err := f.store.LoadAttemptOutcomes(ctx, input.RunID)
	if err != nil {
		return err
	}
	facts, err := f.store.LoadSettlementFacts(ctx, input.RunID)
	if err != nil {
		return err
	}
	if facts.ActualUnits < 0 || facts.HoldUnits < 0 {
		return fmt.Errorf("negative persisted settlement units")
	}
	input.ActualUnits = facts.ActualUnits
	input.HoldUnits = facts.HoldUnits
	input.Items = append([]billing.UsageChargeItem(nil), facts.Items...)
	complete, reason := completeUsage(outcomes, input.RunStatus)
	if !complete {
		input.BillingStatus = billing.BillingStatusUnbilled
		if reason == string(billing.BillingReasonReleasedProviderFailed) || reason == string(billing.BillingReasonReleasedOutcomeUnknown) {
			input.BillingStatus = billing.BillingStatusReleased
		}
		input.BillingReason = billing.BillingReason(reason)
		_, err := f.store.Release(ctx, input)
		return err
	}
	if input.ActualUnits > input.HoldUnits {
		input.BillingStatus = billing.BillingStatusUnbilled
		input.BillingReason = billing.BillingReasonUnbilledOverHold
		_, err := f.store.Release(ctx, input)
		return err
	}
	if input.ActualUnits == 0 {
		input.BillingStatus = billing.BillingStatusSettled
		input.BillingReason = billing.BillingReasonSettledCompleteUsage
		// There is no wallet capture to record for a zero-value settlement;
		// release only the reservation while persisting the settled Run/Charge.
		_, err := f.store.Release(ctx, input)
		return err
	}
	input.BillingStatus = billing.BillingStatusSettled
	input.BillingReason = billing.BillingReasonSettledCompleteUsage
	_, err = f.store.CaptureAndRelease(ctx, input)
	return err
}

func completeUsage(outcomes []AttemptOutcome, runStatus string) (bool, string) {
	if len(outcomes) == 0 && strings.TrimSpace(runStatus) == "failed" {
		return false, string(billing.BillingReasonReleasedProviderFailed)
	}
	hasBillable := false
	hasFailed := false
	for _, outcome := range outcomes {
		switch strings.TrimSpace(outcome.State) {
		case string(billing.AttemptStateFailed):
			// Failed attempts are audit-only and never block a later success.
			hasFailed = true
			continue
		case string(billing.AttemptStateSucceeded):
			hasBillable = true
			if !outcome.UsageComplete && !outcome.Usage.Complete() {
				return false, string(billing.BillingReasonUnbilledUsageIncomplete)
			}
		case string(billing.AttemptStateDispatched), string(billing.AttemptStateCanceled):
			if strings.TrimSpace(runStatus) == "canceled" && (!outcome.UsageComplete && !outcome.Usage.Complete()) {
				return false, string(billing.BillingReasonUnbilledUsageIncomplete)
			}
		case string(billing.AttemptStateOutcomeUnknown):
			return false, string(billing.BillingReasonReleasedOutcomeUnknown)
		default:
			return false, string(billing.BillingReasonUnbilledUsageIncomplete)
		}
	}
	if !hasBillable && hasFailed {
		return false, string(billing.BillingReasonReleasedProviderFailed)
	}
	return true, string(billing.BillingReasonSettledCompleteUsage)
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
