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
			if _, err := g.deps.Runs.LockRunAndCharge(ctx, tx, input.RunID); err != nil {
				return err
			}
			attempt, err := g.deps.Attempts.GetPrepared(ctx, input.RunID, input.AttemptNo)
			if err != nil {
				return err
			}
			if err := g.deps.Reserve.ReserveOrTopUp(ctx, tx, input.RunID, attempt.Quote.TargetHoldUnits); err != nil {
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
	reserve := func(tx Transaction) error {
		g.record("reserve_wallet_hold")
		if g.deps.Reserve == nil {
			return nil
		}
		return g.deps.Reserve.ReserveOrTopUp(ctx, tx, input.RunID, call.Quote.TargetHoldUnits)
	}
	var prepared ProviderAttempt
	attempt := ProviderAttempt{RunID: input.RunID, AttemptNo: input.AttemptNo, IdempotencyKey: attemptKey(input.RunID, input.AttemptNo), PreparedRequest: append([]byte(nil), call.RequestBody...), RequestSHA256: call.RequestSHA256, Quote: call.Quote}
	txErr := g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
		if _, err := g.deps.Runs.LockRunAndCharge(ctx, tx, input.RunID); err != nil {
			return err
		}
		if existing, err := g.deps.Attempts.GetPrepared(ctx, input.RunID, input.AttemptNo); err == nil {
			if !sameAttemptEvidence(existing, attempt) {
				return gatewayError(ErrCodeDuplicateAttempt, "attempt evidence differs from persisted evidence", 409)
			}
			if err := reserve(tx); err != nil {
				return err
			}
			prepared = cloneAttempt(existing)
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := reserve(tx); err != nil {
			return err
		}
		g.record("persist_prepared")
		if err := g.deps.Attempts.PutPrepared(ctx, tx, attempt); err != nil {
			return err
		}
		prepared = cloneAttempt(attempt)
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
		if _, err := g.deps.Runs.LockRunAndCharge(ctx, tx, attempt.RunID); err != nil {
			return err
		}
		persisted, err := g.deps.Attempts.GetPrepared(ctx, attempt.RunID, attempt.AttemptNo)
		if err != nil {
			return gatewayError(ErrCodePreparedMissing, "prepared attempt does not exist", 409)
		}
		if !sameAttemptEvidence(persisted, attempt) {
			return gatewayError(ErrCodeDuplicateAttempt, "provider attempt evidence differs from persisted attempt", 409)
		}
		if err := g.deps.Reserve.EnsureActiveHold(ctx, tx, attempt.RunID, persisted.Quote.TargetHoldUnits); err != nil {
			return err
		}
		if err := g.deps.Owner.EnsureRunnable(ctx, tx, attempt.RunID); err != nil {
			return err
		}
		return g.deps.Attempts.MarkDispatched(ctx, tx, attempt.RunID, attempt.AttemptNo)
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
		return DispatchResult{DispatchState: infraai.DispatchStateUnknown, TerminalState: "unknown"}, ErrNotConfigured
	}
	result, err := g.deps.Provider.Dispatch(ctx, attempt)
	if err != nil {
		if result.DispatchState == "" {
			result.DispatchState = infraai.DispatchStateUnknown
		}
		if result.TerminalState == "" {
			result.TerminalState = "unknown"
		}
		if recordErr := g.RecordOutcome(ctx, attempt, result); recordErr != nil {
			return result, errors.Join(err, recordErr)
		}
		return result, err
	}
	if result.DispatchState == "" {
		result.DispatchState = infraai.DispatchStateDispatched
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
	if err := g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
		return g.deps.Attempts.RecordOutcome(ctx, tx, attempt.RunID, attempt.AttemptNo, result)
	}); err != nil {
		return err
	}
	return nil
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
