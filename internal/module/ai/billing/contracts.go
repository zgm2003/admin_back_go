package billing

import (
	"context"
	"errors"
	"strings"
	"time"
)

type BillingStatus string
type BillingReason string
type HoldStatus string
type ChargeStatus string
type UsageStatus string
type UsageCategory string
type AttemptState string
type DispatchState string
type SHA256Digest [32]byte

const (
	BillingStatusPending  BillingStatus = "pending"
	BillingStatusHeld     BillingStatus = "held"
	BillingStatusSettled  BillingStatus = "settled"
	BillingStatusReleased BillingStatus = "released"
	BillingStatusUnbilled BillingStatus = "unbilled"
)

const (
	BillingReasonPending                     BillingReason = "pending"
	BillingReasonHeld                        BillingReason = "held"
	BillingReasonSettledCompleteUsage        BillingReason = "settled_complete_usage"
	BillingReasonReleasedBeforeDispatch      BillingReason = "released_before_dispatch"
	BillingReasonReleasedInsufficientBalance BillingReason = "released_insufficient_balance"
	BillingReasonReleasedProviderFailed      BillingReason = "released_provider_failed"
	BillingReasonReleasedOutcomeUnknown      BillingReason = "released_outcome_unknown"
	BillingReasonUnbilledUsageIncomplete     BillingReason = "unbilled_usage_incomplete"
	BillingReasonUnbilledOverHold            BillingReason = "unbilled_over_hold"
	BillingReasonLegacyUnpriced              BillingReason = "legacy_unpriced"
)

const (
	HoldStatusActive   HoldStatus = "active"
	HoldStatusCaptured HoldStatus = "captured"
	HoldStatusReleased HoldStatus = "released"
)

const (
	ChargeStatusOpen     ChargeStatus = "open"
	ChargeStatusSettled  ChargeStatus = "settled"
	ChargeStatusReleased ChargeStatus = "released"
	ChargeStatusUnbilled ChargeStatus = "unbilled"
)

const (
	UsageStatusComplete    UsageStatus = "complete"
	UsageStatusUnavailable UsageStatus = "unavailable"
)

const (
	UsageCategoryInputText  UsageCategory = "input"
	UsageCategoryOutputText UsageCategory = "output"
	UsageCategoryCacheRead  UsageCategory = "cache_read"
	UsageCategoryCacheWrite UsageCategory = "cache_write"
	UsageCategoryMedia      UsageCategory = "media"
)

const (
	AttemptStatePrepared       AttemptState = "prepared"
	AttemptStateDispatched     AttemptState = "dispatched"
	AttemptStateSucceeded      AttemptState = "succeeded"
	AttemptStateFailed         AttemptState = "failed"
	AttemptStateCanceled       AttemptState = "canceled"
	AttemptStateOutcomeUnknown AttemptState = "outcome_unknown"
)

const (
	DispatchStateNotDispatched DispatchState = "not_dispatched"
	DispatchStateDispatched    DispatchState = "dispatched"
	DispatchStateUnknown       DispatchState = "unknown"
)

var ErrInvalidUsageItem = errors.New("usage item category, unit and positive quantity are required")

type UsageItem struct {
	Category UsageCategory `json:"category"`
	Unit     string        `json:"unit"`
	TierKey  string        `json:"tier_key"`
	Quantity int64         `json:"quantity"`
}

func (item UsageItem) Validate() error {
	_, err := item.Normalized()
	return err
}

func (item UsageItem) Normalized() (UsageItem, error) {
	item.Unit = strings.TrimSpace(item.Unit)
	item.TierKey = strings.TrimSpace(item.TierKey)
	if !validUsageCategory(item.Category) || item.Unit == "" || item.Quantity <= 0 {
		return UsageItem{}, ErrInvalidUsageItem
	}
	return item, nil
}

func validUsageCategory(category UsageCategory) bool {
	switch category {
	case UsageCategoryInputText, UsageCategoryOutputText, UsageCategoryCacheRead, UsageCategoryCacheWrite, UsageCategoryMedia:
		return true
	default:
		return false
	}
}

func CanTransitionBillingStatus(from, to BillingStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case BillingStatusPending:
		return to == BillingStatusHeld || to == BillingStatusReleased || to == BillingStatusUnbilled
	case BillingStatusHeld:
		return to == BillingStatusSettled || to == BillingStatusReleased || to == BillingStatusUnbilled
	default:
		return false
	}
}

func CanTransitionHoldStatus(from, to HoldStatus) bool {
	return from == to || (from == HoldStatusActive && (to == HoldStatusCaptured || to == HoldStatusReleased))
}

func CanTransitionAttemptState(from, to AttemptState) bool {
	if from == to {
		return true
	}
	switch from {
	case AttemptStatePrepared:
		return to == AttemptStateDispatched || to == AttemptStateCanceled
	case AttemptStateDispatched:
		return to == AttemptStateSucceeded || to == AttemptStateFailed || to == AttemptStateCanceled || to == AttemptStateOutcomeUnknown
	default:
		return false
	}
}

// Tx is an opaque transaction handle implemented by the persistence adapter.
// Orchestrators pass it between participants without depending on GORM.
type Tx interface {
	BillingTx()
}

type AtomicRunner interface {
	WithinBillingTransaction(context.Context, func(Tx) error) error
}

type RunAcceptance struct {
	UserID                int64
	RequestID             string
	RequestFingerprint    [32]byte
	RequestIdentityStatus string
	RequestIdentityMarker string
	PricingSnapshotJSON   string
	BillingStatus         BillingStatus
	BillingReason         BillingReason
	AcceptedAt            time.Time
}

type AcceptedRun struct {
	RunID    int64
	ChargeID int64
}

type RunChargeAccepter interface {
	AcceptRunAndCharge(context.Context, Tx, RunAcceptance) (AcceptedRun, error)
}

type ReserveAndPrepareInput struct {
	RunID                 int64
	WalletID              int64
	UserID                int64
	AttemptNo             uint
	HoldTargetUnits       int64
	ChargeHeldUnits       int64
	IdempotencyKey        string
	PreparedRequestJSON   string
	PreparedRequestSHA256 [32]byte
	QuoteJSON             string
	PreparedAt            time.Time
}

type PreparedAttempt struct {
	AttemptID int64
	HoldID    int64
	ChargeID  int64
}

type ReserveAndPrepareRecorder interface {
	ReserveAndPrepare(context.Context, Tx, ReserveAndPrepareInput) (PreparedAttempt, error)
}

type DispatchEvidence struct {
	AttemptID         int64
	ProviderRequestID string
	DispatchedAt      time.Time
}

type OutcomeEvidence struct {
	AttemptID           int64
	DispatchState       DispatchState
	State               AttemptState
	ProviderRequestID   string
	ResponseSHA256      SHA256Digest
	UsageStatus         UsageStatus
	UsageJSON           string
	ResultCandidateJSON *string
	ErrorCode           string
	FinishedAt          time.Time
}

type AttemptEvidenceRecorder interface {
	RecordDispatched(context.Context, Tx, DispatchEvidence) error
	RecordOutcome(context.Context, Tx, OutcomeEvidence) error
}

type FinalizeInput struct {
	RunID         int64
	RunStatus     string
	BillingStatus BillingStatus
	BillingReason BillingReason
	HoldStatus    HoldStatus
	ChargeStatus  ChargeStatus
	ActualUnits   int64
	Items         []UsageChargeItem
	EventType     string
	FinalizedAt   time.Time
}

type FinalizerParticipant interface {
	FinalizeRunChargeHold(context.Context, Tx, FinalizeInput) error
}
