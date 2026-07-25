package aigateway

import (
	"context"
	"fmt"
	"strings"
	"sync"

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

type FinalizationStore interface {
	LoadAttemptOutcomes(context.Context, int64) ([]AttemptOutcome, error)
	CaptureAndRelease(context.Context, FinalizeInput) error
	Release(context.Context, FinalizeInput) error
}

type RunFinalizer struct {
	store     FinalizationStore
	mu        sync.Mutex
	finalized map[int64]struct{}
}

func NewFinalizer(store FinalizationStore) *RunFinalizer {
	return &RunFinalizer{store: store, finalized: make(map[int64]struct{})}
}

func (f *RunFinalizer) Finalize(ctx context.Context, input FinalizeInput) error {
	if f == nil || f.store == nil {
		return ErrNotConfigured
	}
	f.mu.Lock()
	if _, ok := f.finalized[input.RunID]; ok {
		f.mu.Unlock()
		return nil
	}
	f.mu.Unlock()
	if input.ActualUnits < 0 || input.HoldUnits < 0 {
		return fmt.Errorf("negative settlement units")
	}
	outcomes, err := f.store.LoadAttemptOutcomes(ctx, input.RunID)
	if err != nil {
		return err
	}
	complete, reason := completeUsage(outcomes, input.RunStatus)
	if !complete {
		input.BillingStatus = billing.BillingStatusUnbilled
		if reason == string(billing.BillingReasonReleasedProviderFailed) || reason == string(billing.BillingReasonReleasedOutcomeUnknown) {
			input.BillingStatus = billing.BillingStatusReleased
		}
		input.BillingReason = billing.BillingReason(reason)
		err := f.store.Release(ctx, input)
		if err == nil {
			f.markFinalized(input.RunID)
		}
		return err
	}
	if input.ActualUnits > input.HoldUnits {
		input.BillingStatus = billing.BillingStatusUnbilled
		input.BillingReason = billing.BillingReasonUnbilledOverHold
		err := f.store.Release(ctx, input)
		if err == nil {
			f.markFinalized(input.RunID)
		}
		return err
	}
	if input.ActualUnits == 0 {
		input.BillingStatus = billing.BillingStatusSettled
		input.BillingReason = billing.BillingReasonSettledCompleteUsage
		err := f.store.CaptureAndRelease(ctx, input)
		if err == nil {
			f.markFinalized(input.RunID)
		}
		return err
	}
	input.BillingStatus = billing.BillingStatusSettled
	input.BillingReason = billing.BillingReasonSettledCompleteUsage
	err = f.store.CaptureAndRelease(ctx, input)
	if err == nil {
		f.markFinalized(input.RunID)
	}
	return err
}

func (f *RunFinalizer) markFinalized(runID int64) {
	f.mu.Lock()
	f.finalized[runID] = struct{}{}
	f.mu.Unlock()
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
