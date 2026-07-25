package aigateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type RunRequest struct {
	UserID             int64
	RunID              int64
	RequestID          string
	RequestFingerprint [32]byte
	Modality           string
	Input              []byte
}

type PreparedCall struct {
	RequestBody   []byte
	RequestSHA256 [32]byte
	Quote         QuoteEvidence
}

type QuoteEvidence struct {
	PricingVersion           string              `json:"pricing_version"`
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
	Quote           QuoteEvidence
}

type DispatchResult struct {
	ProviderRequestID string
	DispatchState     string
	TerminalState     string
	Usage             infraai.UsageSnapshot
	UsageComplete     bool
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
	AssembleAndQuote(context.Context, RunRequest) (PreparedCall, error)
}

// Transaction is deliberately opaque. Implementations must pass the same
// transaction to wallet and attempt participants; they must not nest one.
type Transaction interface{ BillingTx() }

type TransactionRunner interface {
	WithinTransaction(context.Context, func(Transaction) error) error
}

type ReserveParticipant interface {
	ReserveOrTopUp(context.Context, Transaction, int64, int64) error
}

type AttemptStore interface {
	PutPrepared(context.Context, Transaction, ProviderAttempt) error
	GetPrepared(context.Context, int64, uint32) (ProviderAttempt, error)
	MarkDispatched(context.Context, Transaction, int64, uint32) error
	RecordOutcome(context.Context, Transaction, int64, uint32, DispatchResult) error
}

type Provider interface {
	Dispatch(context.Context, ProviderAttempt) (DispatchResult, error)
}

type RunStore interface {
	LoadRun(context.Context, int64) (RunSnapshot, error)
	LockRunAndCharge(context.Context, Transaction, int64) (RunSnapshot, error)
	RequestFingerprint(context.Context, int64, string) ([32]byte, error)
}

type FinalizeInput struct {
	RunID         int64
	RunStatus     string
	BillingStatus billing.BillingStatus
	BillingReason billing.BillingReason
	ActualUnits   int64
	HoldUnits     int64
	Items         []billing.UsageChargeItem
	AgentID       int64
	Model         string
	ModelDisplay  string
}

type Finalizer interface {
	Finalize(context.Context, FinalizeInput) error
}

type Dependencies struct {
	Assembler    Assembler
	Transactions TransactionRunner
	Runs         RunStore
	Reserve      ReserveParticipant
	Attempts     AttemptStore
	Provider     Provider
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
	if call.Quote.TargetHoldUnits < 0 || call.Quote.EffectiveMaxOutputTokens < 0 {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "prepared quote contains negative values", 400)
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
