package aigateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

type preparedChatTransportStub struct {
	preparedRequest infraai.PreparedChatRequest
	result          *infraai.ChatResult
	err             error
}

func (*preparedChatTransportStub) TestConnection(context.Context, infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (*preparedChatTransportStub) StreamChat(context.Context, infraai.ChatInput, infraai.EventSink) (*infraai.ChatResult, error) {
	return nil, errors.New("unexpected mutable chat dispatch")
}

func (*preparedChatTransportStub) PrepareChat(context.Context, infraai.ChatInput) ([]byte, error) {
	return nil, errors.New("unexpected reassembly")
}

func (*preparedChatTransportStub) PreflightPreparedChat(context.Context, []byte) error { return nil }

func (s *preparedChatTransportStub) StreamPreparedChat(_ context.Context, request infraai.PreparedChatRequest, _ infraai.EventSink) (*infraai.ChatResult, error) {
	s.preparedRequest = infraai.PreparedChatRequest{Body: append([]byte(nil), request.Body...), IdempotencyKey: request.IdempotencyKey}
	return s.result, s.err
}

func (*preparedChatTransportStub) Capabilities() infraai.CapabilityMetadata {
	return infraai.CapabilityMetadata{
		SupportedUsageIdentities: []infraai.UsageIdentity{
			{Category: infraai.UsageCategoryInput, Unit: "token"},
			{Category: infraai.UsageCategoryOutput, Unit: "token"},
		},
		SafeInputUpperBoundStrategy: infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1,
		SupportsIdempotencyHeader:   true,
	}
}

func TestPreparedChatProviderProofUsesExactRequestBound(t *testing.T) {
	body := []byte(`{"model":"gpt-test","messages":[]}`)
	inputBound, err := infraai.SafeInputUpperBoundFromRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	attempt := ProviderAttempt{
		RunID:           9,
		AttemptNo:       1,
		PreparedRequest: body,
		RequestSHA256:   sha256.Sum256(body),
		Quote: QuoteEvidence{
			EffectiveMaxOutputTokens: 20,
			UpperBoundItems: []billing.UsageItem{
				{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: inputBound},
				{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 20},
			},
		},
	}
	provider := NewPreparedChatProvider(&preparedChatTransportStub{}, nil, nil)
	proof, err := provider.ProvePreparedUpperBound(context.Background(), attempt)
	if err != nil {
		t.Fatalf("ProvePreparedUpperBound: %v", err)
	}
	if proof.RequestSHA256 != attempt.RequestSHA256 || proof.Strategy != infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1 || len(proof.Items) != 2 || proof.Items[0].Quantity != inputBound {
		t.Fatalf("proof=%+v", proof)
	}
	attempt.Quote.UpperBoundItems[0].Quantity--
	if _, err := provider.ProvePreparedUpperBound(context.Background(), attempt); err == nil {
		t.Fatal("expected a mismatched input quote to fail closed")
	}
}

func TestPreparedChatProviderDispatchesPersistedBytesAndReturnsCandidate(t *testing.T) {
	body := []byte("{ \n  \"model\": \"gpt-test\", \"messages\": []\n}")
	rawUsage := []byte(`{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}`)
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusComplete, rawUsage, []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 2},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	responseHash := sha256.Sum256([]byte("provider-response"))
	transport := &preparedChatTransportStub{result: &infraai.ChatResult{
		ProviderRequestID: "req-1",
		Answer:            "hello",
		Usage:             usage,
		DispatchState:     infraai.DispatchStateDispatched,
		ResponseSHA256:    responseHash,
	}}
	provider := NewPreparedChatProvider(transport, nil, func(result *infraai.ChatResult) (*string, error) {
		candidate := `{"version":"ai_chat_result_v1","answer":"` + result.Answer + `"}`
		return &candidate, nil
	})
	outcome, err := provider.Dispatch(context.Background(), ProviderAttempt{PreparedRequest: body, IdempotencyKey: "attempt-key"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if string(transport.preparedRequest.Body) != string(body) || transport.preparedRequest.IdempotencyKey != "attempt-key" {
		t.Fatalf("prepared request=%+v", transport.preparedRequest)
	}
	if outcome.ProviderRequestID != "req-1" || outcome.ResponseSHA256 != responseHash || outcome.TerminalState != "succeeded" || outcome.ResultCandidateJSON == nil {
		t.Fatalf("outcome=%+v", outcome)
	}
	result := provider.ChatResult()
	if result == nil || result.Answer != "hello" {
		t.Fatalf("chat result=%+v", result)
	}
}
