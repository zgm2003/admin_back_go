package aigateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type Gateway struct {
	deps Dependencies
	now  func() time.Time

	mu    sync.Mutex
	trace []string
}

func New(deps Dependencies) *Gateway {
	return &Gateway{deps: deps, now: time.Now}
}

func (g *Gateway) record(step string) {
	g.mu.Lock()
	g.trace = append(g.trace, step)
	g.mu.Unlock()
}

// OperationTrace is intended for focused ordering tests and diagnostics.
func (g *Gateway) OperationTrace() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.trace...)
}

func (g *Gateway) AssembleAndQuote(ctx context.Context, req RunRequest) (PreparedCall, error) {
	if g == nil {
		return PreparedCall{}, ErrNotConfigured
	}
	if err := requireRunID(req.RunID); err != nil || req.UserID <= 0 || req.RequestID == "" {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "run and request identity are required", 400)
	}
	if g.deps.Runs == nil || g.deps.Assembler == nil {
		return PreparedCall{}, ErrNotConfigured
	}
	persisted, err := g.deps.Runs.LoadRun(ctx, req.RunID)
	if err != nil {
		return PreparedCall{}, err
	}
	if persisted.RunID != req.RunID || persisted.UserID != req.UserID || persisted.RequestID != req.RequestID || persisted.RequestFingerprint != req.RequestFingerprint {
		return PreparedCall{}, gatewayError(ErrCodeFingerprintConflict, "request fingerprint conflicts with persisted run", 409)
	}
	g.record("load_immutable_config")
	g.record("assemble_and_quote")
	call, err := g.deps.Assembler.AssembleAndQuote(ctx, persisted, req)
	if err != nil {
		return PreparedCall{}, err
	}
	return canonicalPrepared(call)
}

func (g *Gateway) ReserveAndPrepare(ctx context.Context, input ReserveAndPrepareInput) (ProviderAttempt, error) {
	if g == nil || g.deps.Transactions == nil || g.deps.Runs == nil || g.deps.Reserve == nil || g.deps.Attempts == nil {
		return ProviderAttempt{}, ErrNotConfigured
	}
	if err := requireRunID(input.RunID); err != nil || input.AttemptNo == 0 {
		return ProviderAttempt{}, gatewayError(ErrCodeInvalidPrepared, "run_id and attempt_no are required", 400)
	}
	if input.NewCall == nil {
		g.record("recover_prepared")
		var recovered ProviderAttempt
		err := g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
			locked, err := g.deps.Runs.LockRunAndCharge(ctx, tx, input.RunID)
			if err != nil {
				return err
			}
			if err := validateLockedRunCharge(input.RunID, locked); err != nil {
				return err
			}
			attempt, err := g.deps.Attempts.GetPreparedForUpdate(ctx, tx, input.RunID, input.AttemptNo)
			if err != nil {
				return err
			}
			facts, err := g.deps.Reserve.ReserveOrTopUp(ctx, tx, input.RunID, attempt.Quote.TargetHoldUnits)
			if err != nil {
				return err
			}
			if err := validateBillingFacts(input.RunID, attempt.Quote.TargetHoldUnits, locked.ChargeHeldAuditMax, facts); err != nil {
				return err
			}
			recovered = cloneAttempt(attempt)
			return nil
		})
		if err != nil {
			if isInsufficient(err) {
				return ProviderAttempt{}, gatewayError(ErrCodeInsufficientBalance, err.Error(), 409)
			}
			return ProviderAttempt{}, err
		}
		return recovered, nil
	}
	call, err := canonicalPrepared(*input.NewCall)
	if err != nil {
		return ProviderAttempt{}, err
	}
	g.record("lock_run")
	g.record("lock_charge")
	reserve := func(tx Transaction, locked LockedRunCharge, target int64) error {
		g.record("reserve_wallet_hold")
		facts, err := g.deps.Reserve.ReserveOrTopUp(ctx, tx, input.RunID, target)
		if err != nil {
			return err
		}
		return validateBillingFacts(input.RunID, target, locked.ChargeHeldAuditMax, facts)
	}
	var prepared ProviderAttempt
	attempt := ProviderAttempt{RunID: input.RunID, AttemptNo: input.AttemptNo, IdempotencyKey: attemptKey(input.RunID, input.AttemptNo), PreparedRequest: append([]byte(nil), call.RequestBody...), RequestSHA256: call.RequestSHA256, Quote: call.Quote}
	txErr := g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
		locked, err := g.deps.Runs.LockRunAndCharge(ctx, tx, input.RunID)
		if err != nil {
			return err
		}
		if err := validateLockedRunCharge(input.RunID, locked); err != nil {
			return err
		}
		if existing, err := g.deps.Attempts.GetPreparedForUpdate(ctx, tx, input.RunID, input.AttemptNo); err == nil {
			if !sameAttemptEvidence(existing, attempt) {
				return gatewayError(ErrCodeDuplicateAttempt, "attempt evidence differs from persisted evidence", 409)
			}
			if err := reserve(tx, locked, existing.Quote.TargetHoldUnits); err != nil {
				return err
			}
			prepared = cloneAttempt(existing)
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := reserve(tx, locked, attempt.Quote.TargetHoldUnits); err != nil {
			return err
		}
		g.record("persist_prepared")
		write, err := g.deps.Attempts.PutPrepared(ctx, tx, attempt)
		if err != nil {
			return err
		}
		if !sameAttemptEvidence(write.Attempt, attempt) {
			return gatewayError(ErrCodeDuplicateAttempt, "prepared write evidence differs from requested evidence", 409)
		}
		prepared = cloneAttempt(write.Attempt)
		return nil
	})
	if txErr != nil {
		if isInsufficient(txErr) {
			return ProviderAttempt{}, gatewayError(ErrCodeInsufficientBalance, txErr.Error(), 409)
		}
		return ProviderAttempt{}, txErr
	}
	return prepared, nil
}

func (g *Gateway) MarkDispatched(ctx context.Context, attempt ProviderAttempt) error {
	if g == nil || g.deps.Transactions == nil || g.deps.Runs == nil || g.deps.Reserve == nil || g.deps.Attempts == nil || g.deps.Owner == nil {
		return ErrNotConfigured
	}
	g.record("mark_dispatched")
	err := g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
		locked, err := g.deps.Runs.LockRunAndCharge(ctx, tx, attempt.RunID)
		if err != nil {
			return err
		}
		if err := validateLockedRunCharge(attempt.RunID, locked); err != nil {
			return err
		}
		persisted, err := g.deps.Attempts.GetPreparedForUpdate(ctx, tx, attempt.RunID, attempt.AttemptNo)
		if err != nil {
			return gatewayError(ErrCodePreparedMissing, "prepared attempt does not exist", 409)
		}
		if !sameAttemptEvidence(persisted, attempt) {
			return gatewayError(ErrCodeDuplicateAttempt, "provider attempt evidence differs from persisted attempt", 409)
		}
		facts, err := g.deps.Reserve.EnsureActiveHold(ctx, tx, attempt.RunID, persisted.Quote.TargetHoldUnits)
		if err != nil {
			return err
		}
		if err := validateBillingFacts(attempt.RunID, persisted.Quote.TargetHoldUnits, locked.ChargeHeldAuditMax, facts); err != nil {
			return err
		}
		if err := g.deps.Owner.EnsureRunnable(ctx, tx, attempt.RunID); err != nil {
			return err
		}
		changed, err := g.deps.Attempts.MarkDispatched(ctx, tx, attempt.RunID, attempt.AttemptNo)
		if err != nil {
			return err
		}
		if !changed {
			return gatewayError(ErrCodePreparedMissing, "prepared attempt was not transitioned to dispatched", 409)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (g *Gateway) Dispatch(ctx context.Context, attempt ProviderAttempt) (DispatchResult, error) {
	if err := g.MarkDispatched(ctx, attempt); err != nil {
		return DispatchResult{}, err
	}
	g.record("provider_dispatch")
	if g.deps.Provider == nil {
		return DispatchResult{}, ErrNotConfigured
	}
	result, err := g.deps.Provider.Dispatch(ctx, attempt)
	if err != nil {
		result = terminalResultForProviderError(result, err)
		if recordErr := g.RecordOutcome(ctx, attempt, result); recordErr != nil {
			return result, errors.Join(err, recordErr)
		}
		return result, err
	}
	if err := g.RecordOutcome(ctx, attempt, result); err != nil {
		return DispatchResult{}, err
	}
	return result, nil
}

func (g *Gateway) RecordOutcome(ctx context.Context, attempt ProviderAttempt, result DispatchResult) error {
	if g == nil || g.deps.Transactions == nil || g.deps.Attempts == nil {
		return ErrNotConfigured
	}
	g.record("record_outcome")
	if err := validateTerminalOutcome(result); err != nil {
		return err
	}
	if err := g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
		persisted, err := g.deps.Attempts.GetDispatchedForUpdate(ctx, tx, attempt.RunID, attempt.AttemptNo)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return err
			}
			terminal, terminalErr := g.deps.Attempts.GetTerminalOutcome(ctx, tx, attempt.RunID, attempt.AttemptNo)
			if terminalErr != nil {
				if !errors.Is(terminalErr, ErrNotFound) {
					return terminalErr
				}
				return gatewayError(ErrCodeInvalidOutcome, "dispatched attempt does not exist", 409)
			}
			if !sameTerminalEvidence(terminal, result) {
				return gatewayError(ErrCodeDuplicateAttempt, "terminal outcome differs from persisted outcome", 409)
			}
			return nil
		}
		if !sameAttemptEvidence(persisted, attempt) {
			return gatewayError(ErrCodeDuplicateAttempt, "provider attempt evidence differs from persisted attempt", 409)
		}
		write, err := g.deps.Attempts.RecordTerminalOutcome(ctx, tx, attempt.RunID, attempt.AttemptNo, result)
		if err != nil {
			return err
		}
		if !sameTerminalEvidence(write.Outcome, result) {
			return gatewayError(ErrCodeDuplicateAttempt, "terminal outcome write differs from requested outcome", 409)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func validateLockedRunCharge(runID int64, locked LockedRunCharge) error {
	if locked.Run.RunID != runID || locked.ChargeHeldAuditMax < 0 {
		return gatewayError(ErrCodeInvalidPrepared, "locked run and charge facts are inconsistent", 409)
	}
	return nil
}

func validateBillingFacts(runID, target, minimumAudit int64, facts LockedBillingFacts) error {
	if facts.RunID != runID || facts.HoldTargetUnits != target || facts.ChargeHeldUnits != target || !facts.HoldActive || facts.ChargeHeldAuditMax < facts.ChargeHeldUnits || facts.ChargeHeldAuditMax < minimumAudit {
		return gatewayError(ErrCodeInvalidPrepared, "locked charge and hold facts are inconsistent", 409)
	}
	return nil
}

func terminalResultForProviderError(result DispatchResult, err error) DispatchResult {
	if result.ProviderRequestID == "" {
		result.ProviderRequestID = infraai.ProviderRequestIDFromError(err)
	}
	result.Usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	switch outcome, ok := infraai.ProviderOutcomeFromError(err); {
	case ok && outcome == infraai.ProviderOutcomeNotDispatched:
		result.DispatchState = infraai.DispatchStateNotDispatched
		result.TerminalState = "failed"
	case ok && outcome == infraai.ProviderOutcomeRejected:
		result.DispatchState = infraai.DispatchStateDispatched
		result.TerminalState = "failed"
	default:
		result.DispatchState = infraai.DispatchStateUnknown
		result.TerminalState = "outcome_unknown"
	}
	return result
}

func validateTerminalOutcome(result DispatchResult) error {
	if !validTerminalState(result.TerminalState) || !validDispatchState(result.DispatchState) {
		return gatewayError(ErrCodeInvalidOutcome, "terminal and dispatch states are required", 400)
	}
	if err := result.Usage.Validate(); err != nil {
		return gatewayError(ErrCodeInvalidOutcome, err.Error(), 400)
	}
	hasResponseHash := result.ResponseSHA256 != ([32]byte{})
	if hasResponseHash && result.DispatchState == infraai.DispatchStateNotDispatched {
		return gatewayError(ErrCodeInvalidOutcome, "not-dispatched outcome cannot have response evidence", 400)
	}
	if result.Usage.ResponseSHA256 != ([32]byte{}) && hasResponseHash && result.Usage.ResponseSHA256 != result.ResponseSHA256 {
		return gatewayError(ErrCodeInvalidOutcome, "usage and response hashes differ", 409)
	}
	switch result.TerminalState {
	case "succeeded":
		if result.DispatchState != infraai.DispatchStateDispatched || !result.Usage.Complete() || strings.TrimSpace(result.ProviderRequestID) == "" || !hasResponseHash {
			return gatewayError(ErrCodeInvalidOutcome, "succeeded outcome requires dispatched complete provider evidence", 400)
		}
	case "failed", "canceled":
		if result.Usage.Complete() || result.Usage.Status != infraai.UsageStatusUnavailable {
			return gatewayError(ErrCodeInvalidOutcome, "failed or canceled outcome cannot contain billable usage", 400)
		}
	case "outcome_unknown":
		if result.DispatchState != infraai.DispatchStateUnknown || result.Usage.Complete() || result.Usage.Status != infraai.UsageStatusUnavailable {
			return gatewayError(ErrCodeInvalidOutcome, "unknown outcome requires unknown dispatch and unavailable usage", 400)
		}
	}
	return nil
}

func validTerminalState(state string) bool {
	switch strings.TrimSpace(state) {
	case "succeeded", "failed", "canceled", "outcome_unknown":
		return true
	default:
		return false
	}
}

func validDispatchState(state string) bool {
	switch strings.TrimSpace(state) {
	case infraai.DispatchStateNotDispatched, infraai.DispatchStateDispatched, infraai.DispatchStateUnknown:
		return true
	default:
		return false
	}
}

func sameTerminalEvidence(left, right DispatchResult) bool {
	return left.ProviderRequestID == right.ProviderRequestID &&
		left.ResponseSHA256 == right.ResponseSHA256 &&
		left.DispatchState == right.DispatchState &&
		left.TerminalState == right.TerminalState &&
		reflect.DeepEqual(left.Usage, right.Usage)
}

func (g *Gateway) Finalize(ctx context.Context, input FinalizeInput) error {
	if g == nil {
		return ErrNotConfigured
	}
	g.record("finalize_lock_run")
	g.record("finalize_lock_charge")
	if g.deps.Finalizer == nil {
		return ErrNotConfigured
	}
	return g.deps.Finalizer.Finalize(ctx, input)
}

func attemptKey(runID int64, attemptNo uint32) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("ai-provider:%d:%d", runID, attemptNo)))
	return fmt.Sprintf("%x", sum[:])
}

func cloneAttempt(a ProviderAttempt) ProviderAttempt {
	a.PreparedRequest = append([]byte(nil), a.PreparedRequest...)
	a.Quote.UpperBoundItems = append([]billing.UsageItem(nil), a.Quote.UpperBoundItems...)
	return a
}

func equalQuoteEvidence(left, right QuoteEvidence) bool {
	return left.PricingVersion == right.PricingVersion &&
		left.EffectiveMaxOutputTokens == right.EffectiveMaxOutputTokens &&
		left.TargetHoldUnits == right.TargetHoldUnits &&
		reflect.DeepEqual(left.UpperBoundItems, right.UpperBoundItems)
}

func sameAttemptEvidence(left, right ProviderAttempt) bool {
	return left.RunID == right.RunID && left.AttemptNo == right.AttemptNo &&
		left.IdempotencyKey == right.IdempotencyKey &&
		bytes.Equal(left.PreparedRequest, right.PreparedRequest) &&
		left.RequestSHA256 == right.RequestSHA256 &&
		equalQuoteEvidence(left.Quote, right.Quote)
}

func isInsufficient(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*Error); ok {
		return e.Code == ErrCodeInsufficientBalance
	}
	return strings.Contains(strings.ToLower(err.Error()), "insufficient")
}
