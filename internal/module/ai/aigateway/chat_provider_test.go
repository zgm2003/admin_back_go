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
	preparedRequest  infraai.PreparedChatRequest
	preflightMetrics *infraai.FileInputMetrics
	result           *infraai.ChatResult
	err              error
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

func (s *preparedChatTransportStub) PreflightPreparedChat(context.Context, []byte) (*infraai.FileInputMetrics, error) {
	return s.preflightMetrics, nil
}

func (s *preparedChatTransportStub) StreamPreparedChat(_ context.Context, request infraai.PreparedChatRequest, _ infraai.EventSink) (*infraai.ChatResult, error) {
	s.preparedRequest = infraai.PreparedChatRequest{Body: append([]byte(nil), request.Body...), IdempotencyKey: request.IdempotencyKey}
	return s.result, s.err
}

func TestPreparedChatProviderMergesPreflightHeadMetricsIntoDispatchResult(t *testing.T) {
	transport := &preparedChatTransportStub{
		preflightMetrics: &infraai.FileInputMetrics{COSHeadMS: 17},
		result: &infraai.ChatResult{FileInputMetrics: &infraai.FileInputMetrics{
			COSStreamMS: 23, MaterializedRequestBytes: 41,
		}},
	}
	provider := NewPreparedChatProvider(transport, nil, nil)
	if err := provider.PreflightPrepared(context.Background(), ProviderAttempt{}); err != nil {
		t.Fatal(err)
	}
	dispatch, err := provider.Dispatch(context.Background(), ProviderAttempt{})
	if err != nil {
		t.Fatal(err)
	}
	if dispatch.FileInputMetrics == nil || dispatch.FileInputMetrics.COSHeadMS != 17 || dispatch.FileInputMetrics.COSStreamMS != 23 || dispatch.FileInputMetrics.MaterializedRequestBytes != 41 {
		t.Fatalf("dispatch metrics=%#v", dispatch.FileInputMetrics)
	}
	metrics := provider.ChatResult().FileInputMetrics
	if metrics == nil || metrics.COSHeadMS != 17 || metrics.COSStreamMS != 23 || metrics.MaterializedRequestBytes != 41 {
		t.Fatalf("metrics=%#v", metrics)
	}
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

func TestPreparedChatProviderProofUsesExactResponsesEnvelopeBound(t *testing.T) {
	body, err := infraai.MarshalPreparedChatInlineEnvelope(infraai.PreparedChatInlineEnvelope{
		Schema:      infraai.PreparedChatSchemaResponsesInlineV1,
		APIProtocol: infraai.APIProtocolResponses,
		Request:     []byte(`{"model":"gpt-test","input":[],"stream":true,"store":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
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
			InputUpperBoundStrategy:  infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1,
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
	if proof.Strategy != infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1 || len(proof.Items) != 2 || proof.Items[0].Quantity != inputBound {
		t.Fatalf("proof=%+v", proof)
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

func TestPreparedChatProviderPreservesTerminalResultEvidenceOnError(t *testing.T) {
	rawUsage := []byte(`{"input_tokens":4,"output_tokens":1,"total_tokens":5}`)
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusComplete, rawUsage, []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 4},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	responseHash := sha256.Sum256([]byte("terminal-response"))
	providerErr := infraai.NewProviderError(infraai.ProviderOutcomeRejected, "req-terminal", errors.New("terminal failure"))
	transport := &preparedChatTransportStub{
		preflightMetrics: &infraai.FileInputMetrics{COSHeadMS: 7},
		result: &infraai.ChatResult{
			ProviderRequestID: "req-terminal", DispatchState: infraai.DispatchStateDispatched,
			ResponseSHA256: responseHash, Usage: usage,
			FileInputMetrics: &infraai.FileInputMetrics{COSStreamMS: 11, MaterializedRequestBytes: 128},
			Continuation: &infraai.ChatContinuation{
				Protocol: infraai.APIProtocolResponses,
				Items:    []byte(`[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]`),
			},
		},
		err: providerErr,
	}
	provider := NewPreparedChatProvider(transport, nil, nil)
	if err := provider.PreflightPrepared(context.Background(), ProviderAttempt{}); err != nil {
		t.Fatal(err)
	}
	outcome, err := provider.Dispatch(context.Background(), ProviderAttempt{})
	if !errors.Is(err, providerErr) {
		t.Fatalf("Dispatch error=%v", err)
	}
	if outcome.ProviderRequestID != "req-terminal" || outcome.ResponseSHA256 != responseHash || !outcome.Usage.Complete() ||
		outcome.FileInputMetrics == nil || outcome.FileInputMetrics.COSHeadMS != 7 || outcome.FileInputMetrics.COSStreamMS != 11 {
		t.Fatalf("terminal outcome evidence=%+v", outcome)
	}
	first := provider.ChatResult()
	if first == nil || first.Continuation == nil {
		t.Fatalf("terminal chat result=%+v", first)
	}
	first.Continuation.Items[0] = 'x'
	second := provider.ChatResult()
	if second == nil || second.Continuation == nil || second.Continuation.Items[0] != '[' {
		t.Fatalf("continuation clone leaked mutation: %+v", second)
	}
}
