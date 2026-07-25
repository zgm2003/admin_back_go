package aigateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/requestidentity"
)

type RunRequest struct {
	UserID    int64
	RunID     int64
	RequestID string
	Identity  requestidentity.Input
}

type PreparedCall struct {
	RequestBody         []byte
	RequestSHA256       [32]byte
	Quote               QuoteEvidence
	assemblyRunID       int64
	assemblyFingerprint [32]byte
	assemblySeal        [32]byte
}

type QuoteEvidence struct {
	PricingVersion           string              `json:"pricing_version"`
	RequestFingerprint       [32]byte            `json:"request_fingerprint"`
	PreparedRequestSHA256    [32]byte            `json:"prepared_request_sha256"`
	EffectiveMaxOutputTokens int                 `json:"effective_max_output_tokens"`
	UpperBoundItems          []billing.UsageItem `json:"upper_bound_items"`
	TargetHoldUnits          int64               `json:"target_hold_units"`
}

type ReserveAndPrepareInput struct {
	RunID     int64
	AttemptNo uint32
	NewCall   *PreparedCall
}

type ProviderAttempt struct {
	RunID           int64
	AttemptNo       uint32
	IdempotencyKey  string
	PreparedRequest []byte
	RequestSHA256   [32]byte
	Quote           QuoteEvidence
}

type DispatchResult struct {
	ProviderRequestID string
	ResponseSHA256    [32]byte
	DispatchState     string
	TerminalState     string
	Usage             infraai.UsageSnapshot
}

type RunSnapshot struct {
	RunID               int64
	UserID              int64
	RequestID           string
	RequestFingerprint  [32]byte
	PricingSnapshotJSON string
	BillingStatus       billing.BillingStatus
	BillingReason       billing.BillingReason
	AgentID             int64
	ModelID             string
	ModelDisplayName    string
}

type Assembler interface {
	AssembleAndQuote(context.Context, RunSnapshot, RunRequest) (PreparedCall, error)
}

// QuoteValidator binds quote evidence to both the exact prepared request and
// the locked immutable pricing snapshot. New calls and recovery use the same
// validator.
type QuoteValidator interface {
	ValidateQuote(context.Context, RunSnapshot, [32]byte, QuoteEvidence) error
}

// Transaction is deliberately opaque. Implementations must pass the same
// transaction to wallet and attempt participants; they must not nest one.
type Transaction interface{ BillingTx() }

type TransactionRunner interface {
	WithinTransaction(context.Context, func(Transaction) error) error
}

// LockedRunCharge is the authoritative run and charge state read with the
// transaction's lock. ChargeHeldAuditMax is the monotonic audit maximum before
// the reserve participant brings the active hold to its target.
type LockedRunCharge struct {
	Run                RunSnapshot
	ChargeHeldAuditMax int64
	HoldTargetUnits    int64
}

// LockedBillingFacts is returned only after reserve/hold work has completed in
// the supplied transaction. The gateway validates this instead of trusting a
// no-result mutation or a RowsAffected side channel.
type LockedBillingFacts struct {
	RunID              int64
	ChargeHeldUnits    int64
	ChargeHeldAuditMax int64
	HoldTargetUnits    int64
	HoldActive         bool
}

type ReserveParticipant interface {
	// ReserveOrTopUp must atomically create or replay the target hold. Returned
	// facts express the operation's affected-row/idempotent result and are
	// verified by Gateway before prepared evidence is persisted.
	ReserveOrTopUp(context.Context, Transaction, int64, int64) (LockedBillingFacts, error)
	// EnsureActiveHold returns the locked owner-scoped hold facts; it must not
	// merely report success from an unverified update.
	EnsureActiveHold(context.Context, Transaction, int64, int64) (LockedBillingFacts, error)
}

type ReserveFailureRecorder interface {
	RecordReserveFailure(context.Context, Transaction, int64, FinalizationTrigger) error
}

type PreparedWriteResult struct {
	Attempt  ProviderAttempt
	Inserted bool
}

type TerminalOutcomeWriteResult struct {
	Outcome  DispatchResult
	Replayed bool
}

type AttemptStore interface {
	// PutPrepared must atomically insert or replay exactly equal evidence. An
	// implementation reports whether it inserted a row rather than hiding
	// RowsAffected/idempotent replay behavior behind a nil error.
	PutPrepared(context.Context, Transaction, ProviderAttempt) (PreparedWriteResult, error)
	GetPreparedForUpdate(context.Context, Transaction, int64, uint32) (ProviderAttempt, error)
	// MarkDispatched returns false when the locked prepared row was not
	// transitioned, including an idempotent/stale replay.
	MarkDispatched(context.Context, Transaction, int64, uint32) (bool, error)
	GetDispatchedForUpdate(context.Context, Transaction, int64, uint32) (ProviderAttempt, error)
	GetTerminalOutcome(context.Context, Transaction, int64, uint32) (DispatchResult, error)
	// RecordTerminalOutcome atomically writes a terminal row or returns its
	// exactly equal terminal evidence with Replayed set.
	RecordTerminalOutcome(context.Context, Transaction, int64, uint32, DispatchResult) (TerminalOutcomeWriteResult, error)
}

// OwnerGuard verifies the command/task lease and absence of cancel intent in
// the same transaction that transitions an attempt to dispatched.
type OwnerGuard interface {
	EnsureRunnable(context.Context, Transaction, int64) error
}

type PreparedUpperBoundProof struct {
	RequestSHA256 [32]byte
	Strategy      string
	Items         []billing.UsageItem
}

type Provider interface {
	infraai.CapabilityProvider
	ProvePreparedUpperBound(context.Context, ProviderAttempt) (PreparedUpperBoundProof, error)
	Dispatch(context.Context, ProviderAttempt) (DispatchResult, error)
}

type RunStore interface {
	LoadRun(context.Context, int64) (RunSnapshot, error)
	LockRunAndCharge(context.Context, Transaction, int64) (LockedRunCharge, error)
}

// FinalizeRequest intentionally carries no mutable billing, run, pricing or
// candidate facts. Finalization reads every such fact from its locked store.
type FinalizeRequest struct{ RunID int64 }

type Finalizer interface {
	Finalize(context.Context, FinalizeRequest) error
}

type Dependencies struct {
	Assembler    Assembler
	Quotes       QuoteValidator
	Transactions TransactionRunner
	Runs         RunStore
	Reserve      ReserveParticipant
	Failures     ReserveFailureRecorder
	Attempts     AttemptStore
	Provider     Provider
	Owner        OwnerGuard
	Finalizer    Finalizer
}

type Error struct {
	Code    string
	Message string
	Status  int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

const (
	ErrCodeInsufficientBalance = "ai.billing.insufficient_balance"
	ErrCodeFingerprintConflict = "ai.billing.request_fingerprint_conflict"
	ErrCodePreparedMissing     = "ai.billing.prepared_attempt_missing"
	ErrCodeInvalidPrepared     = "ai.billing.invalid_prepared_call"
	ErrCodeDuplicateAttempt    = "ai.billing.duplicate_attempt"
	ErrCodeInvalidOutcome      = "ai.billing.invalid_terminal_outcome"
)

var (
	ErrNotConfigured = errors.New("ai gateway dependency not configured")
	ErrNotFound      = errors.New("ai gateway evidence not found")
)

func gatewayError(code, message string, status int) error {
	return &Error{Code: code, Message: message, Status: status}
}

func canonicalPrepared(call PreparedCall) (PreparedCall, error) {
	call.RequestBody = append([]byte(nil), call.RequestBody...)
	if len(call.RequestBody) == 0 {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "prepared request body is required", 400)
	}
	hash := sha256.Sum256(call.RequestBody)
	if call.RequestSHA256 != ([32]byte{}) && call.RequestSHA256 != hash {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "prepared request hash mismatch", 409)
	}
	call.RequestSHA256 = hash
	if call.Quote.PreparedRequestSHA256 != ([32]byte{}) && call.Quote.PreparedRequestSHA256 != hash {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "prepared quote request hash mismatch", 409)
	}
	call.Quote.PreparedRequestSHA256 = hash
	if call.Quote.TargetHoldUnits <= 0 || call.Quote.EffectiveMaxOutputTokens <= 0 || call.Quote.PricingVersion == "" || len(call.Quote.UpperBoundItems) == 0 {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "prepared quote must contain version, capacity, usage, and positive hold", 400)
	}
	for _, item := range call.Quote.UpperBoundItems {
		if err := item.Validate(); err != nil {
			return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, err.Error(), 400)
		}
	}
	return call, nil
}

func quoteJSON(quote QuoteEvidence) string {
	b, _ := json.Marshal(quote)
	return string(b)
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func requireRunID(runID int64) error {
	if runID <= 0 {
		return fmt.Errorf("run_id must be positive")
	}
	return nil
}
