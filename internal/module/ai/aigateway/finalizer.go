package aigateway

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

var (
	ErrFinalizationPending = errors.New("ai finalization facts are not terminal")
	ErrUsageIncomplete     = errors.New("ai settlement usage is incomplete for the persisted catalog")
)

type FinalizationTrigger string

const (
	TriggerSuccess                       FinalizationTrigger = "success"
	TriggerUserStop                      FinalizationTrigger = "user_stop"
	TriggerUserStopBeforeDispatch        FinalizationTrigger = "before_dispatch"
	TriggerInitialInsufficient           FinalizationTrigger = "initial_insufficient"
	TriggerContinuationTopUpInsufficient FinalizationTrigger = "continuation_topup_insufficient"
	TriggerProviderFailed                FinalizationTrigger = "provider_failed"
	TriggerOutcomeUnknown                FinalizationTrigger = "outcome_unknown"
)

type FinalizationCharge struct {
	ID           int64
	HeldUnits    int64
	HeldAuditMax int64
	ActualUnits  int64
	Status       billing.ChargeStatus
}

type FinalizationHold struct {
	ID            int64
	WalletID      int64
	HeldUnits     int64
	HeldAuditMax  int64
	CapturedUnits int64
	Status        billing.HoldStatus
}

type FinalizationAttempt struct {
	ID                int64
	AttemptNo         uint32
	State             billing.AttemptState
	DispatchState     billing.DispatchState
	Usage             infraai.UsageSnapshot
	ProviderRequestID string
}

type FinalizationCandidate struct {
	AttemptID int64
	JSON      string
}

// FinalizationFacts are exclusively loaded under the store's Run -> Charge ->
// wallet -> Hold locks. They are the only facts the decision callback may use.
type FinalizationFacts struct {
	Run              RunSnapshot
	Charge           FinalizationCharge
	Hold             FinalizationHold
	Attempts         []FinalizationAttempt
	Trigger          FinalizationTrigger
	StoppedAttemptID int64
	CurrentAttemptID int64
	Candidate        FinalizationCandidate
}

type BillableAttempt struct {
	ID        int64
	AttemptNo uint32
	Usage     infraai.UsageSnapshot
}

// SettlementPricingInput deliberately carries the persisted pricing snapshot
// and every successful attempt together so the pricer can validate catalog
// completeness and round/allocate once for the whole run.
type SettlementPricingInput struct {
	Run      RunSnapshot
	Attempts []BillableAttempt
}

type SettlementQuote struct {
	ActualUnits int64
	Items       []billing.UsageChargeItem
}

type SettlementPricer interface {
	PriceSettlement(context.Context, SettlementPricingInput) (SettlementQuote, error)
}

type SettlementMoneyAction string

const (
	SettlementMoneyCapture SettlementMoneyAction = "capture"
	SettlementMoneyRelease SettlementMoneyAction = "release"
)

type SettlementCandidateAction string

const (
	SettlementCandidatePublish SettlementCandidateAction = "publish"
	SettlementCandidateDiscard SettlementCandidateAction = "discard"
)

// SettlementDecision lists every persisted finalization effect. In
// particular, capture remains distinct from a nonzero wallet ledger entry:
// stores capture zero-unit settlements but do not create a zero-value entry.
type SettlementDecision struct {
	RunStatus       string
	BillingStatus   billing.BillingStatus
	BillingReason   billing.BillingReason
	ChargeStatus    billing.ChargeStatus
	HoldStatus      billing.HoldStatus
	MoneyAction     SettlementMoneyAction
	CandidateAction SettlementCandidateAction
	Candidate       FinalizationCandidate
	ActualUnits     int64
	HoldUnits       int64
	Items           []billing.UsageChargeItem
	EventType       string
	BillingAnomaly  string
	LedgerSummary   string
}

type FinalizationApplyResult struct {
	Applied  bool
	Replayed bool
}

type FinalizationStore interface {
	// WithLockedSettlement runs one transaction that locks Run, Charge, wallet,
	// then Hold; loads FinalizationFacts; invokes decide; and, guarded by the
	// authoritative terminal/idempotency fence, atomically performs all of:
	// conditional candidate bind or discard, immutable charge-item insertion,
	// wallet capture or release (without a zero-value ledger entry), Hold and
	// Charge status updates, Run status/billing reason update, sequenced billing
	// event insertion, and billing-anomaly insertion. A replay returns Replayed
	// without calling decide or repeating any side effect.
	WithLockedSettlement(context.Context, int64, func(FinalizationFacts) (SettlementDecision, error)) (FinalizationApplyResult, error)
}

type RunFinalizer struct {
	store  FinalizationStore
	pricer SettlementPricer
}

func NewFinalizer(store FinalizationStore, pricer SettlementPricer) *RunFinalizer {
	return &RunFinalizer{store: store, pricer: pricer}
}

func (f *RunFinalizer) Finalize(ctx context.Context, request FinalizeRequest) error {
	if f == nil || f.store == nil || f.pricer == nil || request.RunID <= 0 {
		return ErrNotConfigured
	}
	_, err := f.store.WithLockedSettlement(ctx, request.RunID, func(facts FinalizationFacts) (SettlementDecision, error) {
		if err := validateFinalizationFacts(request.RunID, facts); err != nil {
			return SettlementDecision{}, err
		}
		return f.decide(ctx, facts)
	})
	return err
}

func (f *RunFinalizer) decide(ctx context.Context, facts FinalizationFacts) (SettlementDecision, error) {
	if hasIntermediateAttempt(facts.Attempts) || !validTrigger(facts.Trigger) {
		return SettlementDecision{}, ErrFinalizationPending
	}
	switch facts.Trigger {
	case TriggerSuccess:
		if len(facts.Attempts) == 0 || !allSucceeded(facts.Attempts) {
			return SettlementDecision{}, ErrFinalizationPending
		}
		return f.priceOrUnbilled(ctx, facts, "success", SettlementCandidatePublish)
	case TriggerUserStopBeforeDispatch:
		if hasDispatchedAttempt(facts.Attempts) {
			return SettlementDecision{}, ErrFinalizationPending
		}
		return releaseDecision(facts, "canceled", billing.BillingStatusReleased, billing.BillingReasonReleasedBeforeDispatch), nil
	case TriggerUserStop:
		if len(facts.Attempts) == 0 || !allSucceeded(facts.Attempts) {
			return unbilledDecision(facts, "canceled", SettlementCandidateDiscard), nil
		}
		return f.priceOrUnbilled(ctx, facts, "canceled", SettlementCandidateDiscard)
	case TriggerProviderFailed:
		return releaseDecision(facts, "failed", billing.BillingStatusReleased, billing.BillingReasonReleasedProviderFailed), nil
	case TriggerOutcomeUnknown:
		return releaseDecision(facts, "outcome_unknown", billing.BillingStatusReleased, billing.BillingReasonReleasedOutcomeUnknown), nil
	case TriggerInitialInsufficient:
		return releaseDecision(facts, "failed", billing.BillingStatusReleased, billing.BillingReasonReleasedInsufficientBalance), nil
	case TriggerContinuationTopUpInsufficient:
		if len(successfulAttempts(facts.Attempts)) == 0 {
			return releaseDecision(facts, "failed", billing.BillingStatusReleased, billing.BillingReasonReleasedInsufficientBalance), nil
		}
		return f.priceOrUnbilled(ctx, facts, "failed", SettlementCandidateDiscard)
	default:
		return SettlementDecision{}, ErrFinalizationPending
	}
}

func (f *RunFinalizer) priceOrUnbilled(ctx context.Context, facts FinalizationFacts, runStatus string, candidate SettlementCandidateAction) (SettlementDecision, error) {
	billable := successfulAttempts(facts.Attempts)
	if len(billable) == 0 {
		return unbilledDecision(facts, runStatus, candidate), nil
	}
	quote, err := f.pricer.PriceSettlement(ctx, SettlementPricingInput{Run: facts.Run, Attempts: billable})
	if err != nil {
		if errors.Is(err, ErrUsageIncomplete) {
			return unbilledDecision(facts, runStatus, candidate), nil
		}
		return SettlementDecision{}, err
	}
	items, err := validateSettlementQuote(facts, billable, quote)
	if err != nil {
		return SettlementDecision{}, err
	}
	if quote.ActualUnits > facts.Hold.HeldUnits {
		return overHoldDecision(facts, runStatus, quote, items), nil
	}
	if candidate == SettlementCandidatePublish && !candidateBelongsToBillableAttempt(facts.Candidate, billable) {
		return SettlementDecision{}, errors.New("publish candidate does not belong to a billable attempt")
	}
	return SettlementDecision{
		RunStatus:       runStatus,
		BillingStatus:   billing.BillingStatusSettled,
		BillingReason:   billing.BillingReasonSettledCompleteUsage,
		ChargeStatus:    billing.ChargeStatusSettled,
		HoldStatus:      billing.HoldStatusCaptured,
		MoneyAction:     SettlementMoneyCapture,
		CandidateAction: candidate,
		Candidate:       facts.Candidate,
		ActualUnits:     quote.ActualUnits,
		HoldUnits:       facts.Hold.HeldUnits,
		Items:           items,
		EventType:       "ai.billing.settled",
		LedgerSummary:   LedgerSummary(facts.Run.AgentID, facts.Run.ModelDisplayName, facts.Run.ModelID),
	}, nil
}

func candidateBelongsToBillableAttempt(candidate FinalizationCandidate, billable []BillableAttempt) bool {
	if candidate.AttemptID <= 0 || strings.TrimSpace(candidate.JSON) == "" {
		return false
	}
	for _, attempt := range billable {
		if candidate.AttemptID == attempt.ID {
			return true
		}
	}
	return false
}

func releaseDecision(facts FinalizationFacts, runStatus string, status billing.BillingStatus, reason billing.BillingReason) SettlementDecision {
	return SettlementDecision{
		RunStatus:       runStatus,
		BillingStatus:   status,
		BillingReason:   reason,
		ChargeStatus:    billing.ChargeStatusReleased,
		HoldStatus:      billing.HoldStatusReleased,
		MoneyAction:     SettlementMoneyRelease,
		CandidateAction: SettlementCandidateDiscard,
		Candidate:       facts.Candidate,
		HoldUnits:       facts.Hold.HeldUnits,
		EventType:       "ai.billing.released",
		LedgerSummary:   LedgerSummary(facts.Run.AgentID, facts.Run.ModelDisplayName, facts.Run.ModelID),
	}
}

func unbilledDecision(facts FinalizationFacts, runStatus string, candidate SettlementCandidateAction) SettlementDecision {
	return SettlementDecision{
		RunStatus:       runStatus,
		BillingStatus:   billing.BillingStatusUnbilled,
		BillingReason:   billing.BillingReasonUnbilledUsageIncomplete,
		ChargeStatus:    billing.ChargeStatusUnbilled,
		HoldStatus:      billing.HoldStatusReleased,
		MoneyAction:     SettlementMoneyRelease,
		CandidateAction: candidate,
		Candidate:       facts.Candidate,
		HoldUnits:       facts.Hold.HeldUnits,
		EventType:       "ai.billing.unbilled",
		LedgerSummary:   LedgerSummary(facts.Run.AgentID, facts.Run.ModelDisplayName, facts.Run.ModelID),
	}
}

func overHoldDecision(facts FinalizationFacts, runStatus string, quote SettlementQuote, items []billing.UsageChargeItem) SettlementDecision {
	if runStatus != "canceled" {
		runStatus = "failed"
	}
	decision := unbilledDecision(facts, runStatus, SettlementCandidateDiscard)
	decision.BillingReason = billing.BillingReasonUnbilledOverHold
	decision.ActualUnits = quote.ActualUnits
	decision.Items = items
	decision.BillingAnomaly = "actual usage exceeds authoritative hold"
	decision.EventType = "ai.billing.over_hold"
	return decision
}

func validateFinalizationFacts(runID int64, facts FinalizationFacts) error {
	if facts.Run.RunID != runID || facts.Charge.ID <= 0 || facts.Hold.ID <= 0 || facts.Charge.HeldUnits < 0 || facts.Charge.HeldAuditMax < facts.Charge.HeldUnits || facts.Hold.HeldUnits < 0 || facts.Hold.HeldAuditMax < facts.Hold.HeldUnits || facts.Charge.HeldUnits != facts.Hold.HeldUnits || facts.Charge.HeldAuditMax != facts.Hold.HeldAuditMax {
		return errors.New("locked run, charge, and hold facts are inconsistent")
	}
	return nil
}

func hasIntermediateAttempt(attempts []FinalizationAttempt) bool {
	for _, attempt := range attempts {
		switch attempt.State {
		case billing.AttemptStatePrepared, billing.AttemptStateDispatched:
			return true
		}
	}
	return false
}

func hasDispatchedAttempt(attempts []FinalizationAttempt) bool {
	for _, attempt := range attempts {
		if attempt.DispatchState == billing.DispatchStateDispatched || attempt.State == billing.AttemptStateDispatched {
			return true
		}
	}
	return false
}

func allSucceeded(attempts []FinalizationAttempt) bool {
	for _, attempt := range attempts {
		if attempt.State != billing.AttemptStateSucceeded {
			return false
		}
	}
	return true
}

func successfulAttempts(attempts []FinalizationAttempt) []BillableAttempt {
	billable := make([]BillableAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		if attempt.State == billing.AttemptStateSucceeded {
			billable = append(billable, BillableAttempt{ID: attempt.ID, AttemptNo: attempt.AttemptNo, Usage: attempt.Usage})
		}
	}
	return billable
}

func validTrigger(trigger FinalizationTrigger) bool {
	switch trigger {
	case TriggerSuccess, TriggerUserStop, TriggerUserStopBeforeDispatch, TriggerInitialInsufficient, TriggerContinuationTopUpInsufficient, TriggerProviderFailed, TriggerOutcomeUnknown:
		return true
	default:
		return false
	}
}

func validateSettlementQuote(facts FinalizationFacts, billable []BillableAttempt, quote SettlementQuote) ([]billing.UsageChargeItem, error) {
	if quote.ActualUnits < 0 {
		return nil, errors.New("negative settlement actual units")
	}
	allowed := make(map[int64]struct{}, len(billable))
	for _, attempt := range billable {
		if attempt.ID <= 0 {
			return nil, errors.New("billable attempt identity is invalid")
		}
		allowed[attempt.ID] = struct{}{}
	}
	items := make([]billing.UsageChargeItem, len(quote.Items))
	seen := make(map[string]struct{}, len(items))
	var sum int64
	for i, item := range quote.Items {
		if item.ChargeID != 0 && item.ChargeID != facts.Charge.ID {
			return nil, errors.New("settlement item charge does not match locked charge")
		}
		if _, ok := allowed[item.AttemptID]; !ok {
			return nil, errors.New("settlement item is not from a billable attempt")
		}
		if err := (billing.UsageItem{Category: item.Category, Unit: item.Unit, TierKey: item.TierKey, Quantity: item.Quantity}).Validate(); err != nil || item.UnitPriceUnits < 0 || item.UnitScale <= 0 || item.AmountUnits < 0 {
			return nil, errors.New("settlement item is invalid")
		}
		identity := fmt.Sprintf("%d\x00%s\x00%s\x00%s", item.AttemptID, item.Category, strings.TrimSpace(item.Unit), strings.TrimSpace(item.TierKey))
		if _, exists := seen[identity]; exists {
			return nil, errors.New("duplicate settlement item")
		}
		seen[identity] = struct{}{}
		if item.AmountUnits > math.MaxInt64-sum {
			return nil, errors.New("settlement item amount overflow")
		}
		sum += item.AmountUnits
		item.ChargeID = facts.Charge.ID
		items[i] = item
	}
	if sum != quote.ActualUnits {
		return nil, errors.New("settlement item sum does not equal actual units")
	}
	return items, nil
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
