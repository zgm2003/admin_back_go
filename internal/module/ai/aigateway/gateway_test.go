package aigateway

import (
	"context"
	"errors"
	"testing"
)

type testAssembler struct{ calls int }

func (a *testAssembler) AssembleAndQuote(context.Context, RunRequest) (PreparedCall, error) {
	a.calls++
	body := []byte(`{"model":"gpt-test","max_tokens":10}`)
	return PreparedCall{RequestBody: body, Quote: QuoteEvidence{PricingVersion: "v1", EffectiveMaxOutputTokens: 10, TargetHoldUnits: 25}}, nil
}

type testReserve struct {
	calls int
	err   error
}

func (r *testReserve) ReserveOrTopUp(context.Context, Transaction, int64, int64) error {
	r.calls++
	return r.err
}

func TestGatewayRejectsFingerprintConflictBeforeProvider(t *testing.T) {
	assembler := &testAssembler{}
	gateway := New(Dependencies{Assembler: assembler})
	request := RunRequest{RunID: 1, UserID: 7, RequestID: "r1", RequestFingerprint: [32]byte{1}}
	if _, err := gateway.AssembleAndQuote(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.RequestFingerprint = [32]byte{2}
	var got *Error
	if _, err := gateway.AssembleAndQuote(context.Background(), request); !errors.As(err, &got) || got.Code != ErrCodeFingerprintConflict {
		t.Fatalf("err=%v, want fingerprint conflict", err)
	}
	if assembler.calls != 1 {
		t.Fatalf("assembler calls=%d, want 1", assembler.calls)
	}
}

func TestGatewayReservePersistsBeforeDispatchAndRecoversBytes(t *testing.T) {
	assembler := &testAssembler{}
	reserve := &testReserve{}
	gateway := New(Dependencies{Assembler: assembler, Reserve: reserve})
	call, err := gateway.AssembleAndQuote(context.Background(), RunRequest{RunID: 2, UserID: 7, RequestID: "r2", RequestFingerprint: [32]byte{2}})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 2, AttemptNo: 1, NewCall: &call})
	if err != nil || reserve.calls != 1 {
		t.Fatalf("attempt=%+v reserve_calls=%d err=%v", attempt, reserve.calls, err)
	}
	if err := gateway.MarkDispatched(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 2, AttemptNo: 1, NewCall: nil}); err == nil {
		t.Fatal("dispatched attempt must not be recovered as prepared")
	}
	// A fresh gateway with the same persisted evidence simulates a worker restart.
	prepared := New(Dependencies{})
	prepared.attempts[attemptKey(2, 1)] = &storedAttempt{ProviderAttempt: attempt, State: "prepared"}
	recovered, err := prepared.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 2, AttemptNo: 1})
	if err != nil || string(recovered.PreparedRequest) != string(attempt.PreparedRequest) || recovered.IdempotencyKey != attempt.IdempotencyKey {
		t.Fatalf("recovered=%+v err=%v", recovered, err)
	}
	if assembler.calls != 1 {
		t.Fatalf("prepared recovery assembled again: %d", assembler.calls)
	}
}

func TestGatewayInsufficientBalanceCreatesNoAttempt(t *testing.T) {
	reserve := &testReserve{err: errors.New("insufficient balance")}
	gateway := New(Dependencies{Reserve: reserve})
	body := []byte(`{"model":"gpt-test"}`)
	call := PreparedCall{RequestBody: body, Quote: QuoteEvidence{TargetHoldUnits: 10}}
	_, err := gateway.ReserveAndPrepare(context.Background(), ReserveAndPrepareInput{RunID: 3, AttemptNo: 1, NewCall: &call})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != ErrCodeInsufficientBalance {
		t.Fatalf("err=%v, want insufficient balance", err)
	}
	if len(gateway.attempts) != 0 {
		t.Fatal("insufficient reserve persisted an attempt")
	}
}
