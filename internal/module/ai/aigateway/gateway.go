package aigateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type storedAttempt struct {
	ProviderAttempt
	State  string
	Result *DispatchResult
}

type Gateway struct {
	deps Dependencies
	now  func() time.Time

	mu       sync.Mutex
	attempts map[string]*storedAttempt
	requests map[string][32]byte
	trace    []string
}

func New(deps Dependencies) *Gateway {
	return &Gateway{deps: deps, now: time.Now, attempts: make(map[string]*storedAttempt), requests: make(map[string][32]byte)}
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
	key := fmt.Sprintf("%d:%s", req.UserID, req.RequestID)
	g.mu.Lock()
	if previous, ok := g.requests[key]; ok && previous != req.RequestFingerprint {
		g.mu.Unlock()
		return PreparedCall{}, gatewayError(ErrCodeFingerprintConflict, "request fingerprint conflicts with existing run", 409)
	}
	g.requests[key] = req.RequestFingerprint
	g.mu.Unlock()
	g.record("load_immutable_config")
	if g.deps.Assembler == nil {
		return PreparedCall{}, gatewayError(ErrCodeInvalidPrepared, "no safe input estimator is configured", 422)
	}
	g.record("assemble_and_quote")
	call, err := g.deps.Assembler.AssembleAndQuote(ctx, req)
	if err != nil {
		return PreparedCall{}, err
	}
	return canonicalPrepared(call)
}

func (g *Gateway) ReserveAndPrepare(ctx context.Context, input ReserveAndPrepareInput) (ProviderAttempt, error) {
	if g == nil {
		return ProviderAttempt{}, ErrNotConfigured
	}
	if err := requireRunID(input.RunID); err != nil || input.AttemptNo == 0 {
		return ProviderAttempt{}, gatewayError(ErrCodeInvalidPrepared, "run_id and attempt_no are required", 400)
	}
	key := attemptKey(input.RunID, input.AttemptNo)
	if input.NewCall == nil {
		g.record("recover_prepared")
		if g.deps.Attempts != nil {
			attempt, err := g.deps.Attempts.GetPrepared(ctx, input.RunID, input.AttemptNo)
			if err != nil {
				return ProviderAttempt{}, err
			}
			return attempt, nil
		}
		g.mu.Lock()
		stored, ok := g.attempts[key]
		g.mu.Unlock()
		if !ok || stored.State != "prepared" {
			return ProviderAttempt{}, gatewayError(ErrCodePreparedMissing, "prepared attempt does not exist", 409)
		}
		return cloneAttempt(stored.ProviderAttempt), nil
	}
	call, err := canonicalPrepared(*input.NewCall)
	if err != nil {
		return ProviderAttempt{}, err
	}
	g.mu.Lock()
	if existing, ok := g.attempts[key]; ok {
		if !bytes.Equal(existing.PreparedRequest, call.RequestBody) || existing.Quote.TargetHoldUnits != call.Quote.TargetHoldUnits || existing.Quote.PricingVersion != call.Quote.PricingVersion {
			g.mu.Unlock()
			return ProviderAttempt{}, gatewayError(ErrCodeDuplicateAttempt, "attempt evidence differs from persisted evidence", 409)
		}
		attempt := cloneAttempt(existing.ProviderAttempt)
		g.mu.Unlock()
		return attempt, nil
	}
	g.mu.Unlock()

	g.record("lock_run")
	g.record("lock_charge")
	reserve := func(tx Transaction) error {
		g.record("reserve_wallet_hold")
		if g.deps.Reserve == nil {
			return nil
		}
		return g.deps.Reserve.ReserveOrTopUp(ctx, tx, input.RunID, call.Quote.TargetHoldUnits)
	}
	var txErr error
	attempt := ProviderAttempt{RunID: input.RunID, AttemptNo: input.AttemptNo, IdempotencyKey: key, PreparedRequest: append([]byte(nil), call.RequestBody...), Quote: call.Quote}
	if g.deps.Transactions != nil {
		txErr = g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
			if err := reserve(tx); err != nil {
				return err
			}
			if g.deps.Attempts != nil {
				g.record("persist_prepared")
				return g.deps.Attempts.PutPrepared(ctx, tx, attempt)
			}
			return nil
		})
	} else {
		txErr = reserve(nil)
		if txErr == nil && g.deps.Attempts != nil {
			txErr = g.deps.Attempts.PutPrepared(ctx, nil, attempt)
		}
	}
	if txErr != nil {
		if isInsufficient(txErr) {
			return ProviderAttempt{}, gatewayError(ErrCodeInsufficientBalance, txErr.Error(), 409)
		}
		return ProviderAttempt{}, txErr
	}
	g.mu.Lock()
	g.attempts[key] = &storedAttempt{ProviderAttempt: attempt, State: "prepared"}
	g.mu.Unlock()
	g.record("persist_prepared")
	return attempt, nil
}

func (g *Gateway) MarkDispatched(ctx context.Context, attempt ProviderAttempt) error {
	key := attemptKey(attempt.RunID, attempt.AttemptNo)
	g.mu.Lock()
	stored, ok := g.attempts[key]
	if !ok {
		g.mu.Unlock()
		return gatewayError(ErrCodePreparedMissing, "prepared attempt does not exist", 409)
	}
	if stored.State != "prepared" {
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()
	g.record("mark_dispatched")
	if g.deps.Attempts != nil {
		var err error
		if g.deps.Transactions != nil {
			err = g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
				return g.deps.Attempts.MarkDispatched(ctx, tx, attempt.RunID, attempt.AttemptNo)
			})
		} else {
			err = g.deps.Attempts.MarkDispatched(ctx, nil, attempt.RunID, attempt.AttemptNo)
		}
		if err != nil {
			return err
		}
	}
	g.mu.Lock()
	stored.State = "dispatched"
	g.mu.Unlock()
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
		_ = g.RecordOutcome(ctx, attempt, result)
		return result, err
	}
	if result.DispatchState == "" {
		result.DispatchState = infraai.DispatchStateDispatched
	}
	result.UsageComplete = result.UsageComplete || result.Usage.Complete()
	if err := g.RecordOutcome(ctx, attempt, result); err != nil {
		return DispatchResult{}, err
	}
	return result, nil
}

func (g *Gateway) RecordOutcome(ctx context.Context, attempt ProviderAttempt, result DispatchResult) error {
	key := attemptKey(attempt.RunID, attempt.AttemptNo)
	g.mu.Lock()
	stored, ok := g.attempts[key]
	if !ok {
		g.mu.Unlock()
		return ErrNotFound
	}
	copyResult := result
	stored.Result = &copyResult
	if result.TerminalState != "" {
		stored.State = result.TerminalState
	}
	g.mu.Unlock()
	g.record("record_outcome")
	if g.deps.Attempts == nil {
		return nil
	}
	if g.deps.Transactions != nil {
		return g.deps.Transactions.WithinTransaction(ctx, func(tx Transaction) error {
			return g.deps.Attempts.RecordOutcome(ctx, tx, attempt.RunID, attempt.AttemptNo, result)
		})
	}
	return g.deps.Attempts.RecordOutcome(ctx, nil, attempt.RunID, attempt.AttemptNo, result)
}

func (g *Gateway) Finalize(ctx context.Context, input FinalizeInput) error {
	if g == nil {
		return ErrNotConfigured
	}
	g.record("finalize_lock_run")
	g.record("finalize_lock_charge")
	if input.ActualUnits < 0 || input.HoldUnits < 0 {
		return fmt.Errorf("negative settlement units")
	}
	if input.ActualUnits > input.HoldUnits {
		return gatewayError("ai.billing.unbilled_over_hold", "actual usage exceeds hold", 409)
	}
	if g.deps.Finalizer == nil {
		return nil
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

func isInsufficient(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*Error); ok {
		return e.Code == ErrCodeInsufficientBalance
	}
	return strings.Contains(strings.ToLower(err.Error()), "insufficient")
}
