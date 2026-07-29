package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/openaicompat"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
)

func TestPaidChatAssemblerConvergesPreparedRequestAndSafeOutputBound(t *testing.T) {
	snapshot := paidChatAssemblerPricingSnapshot(t, 300, 100)
	assembler := paidChatAssembler{
		transport: openaicompat.New(openaicompat.Config{}),
		input: infraai.ChatInput{
			Content: "hello",
			Inputs:  map[string]any{"model_id": "forged", "max_tokens": 1},
		},
	}

	call, err := assembler.AssembleAndQuote(context.Background(), aigateway.RunSnapshot{
		RunID: 1, ModelID: "gpt-test", PricingSnapshotJSON: snapshot,
	}, aigateway.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	inputBound, err := infraai.SafeInputUpperBoundFromRequest(call.RequestBody)
	if err != nil {
		t.Fatal(err)
	}
	wantOutputBound := int64(300) - inputBound
	if wantOutputBound > 100 {
		wantOutputBound = 100
	}
	if call.Quote.EffectiveMaxOutputTokens != int(wantOutputBound) {
		t.Fatalf("effective output bound=%d, want %d", call.Quote.EffectiveMaxOutputTokens, wantOutputBound)
	}
	if call.RequestSHA256 != sha256.Sum256(call.RequestBody) || call.Quote.PreparedRequestSHA256 != call.RequestSHA256 {
		t.Fatal("prepared bytes, hash and quote are not bound to the same request")
	}
	if !strings.Contains(string(call.RequestBody), `"max_tokens":`+stringInt(int(wantOutputBound))) || strings.Contains(string(call.RequestBody), `"max_tokens":1,`) {
		t.Fatalf("prepared request did not freeze the converged system bound: %s", call.RequestBody)
	}
}

func TestPaidChatAssemblerFailsClosedWhenBoundDoesNotConverge(t *testing.T) {
	assembler := paidChatAssembler{
		transport: oscillatingPreparedChatTransport{},
		input:     infraai.ChatInput{Content: "hello", Inputs: map[string]any{}},
	}
	_, err := assembler.AssembleAndQuote(context.Background(), aigateway.RunSnapshot{
		RunID: 2, ModelID: "gpt-test", PricingSnapshotJSON: paidChatAssemblerPricingSnapshot(t, 200, 100),
	}, aigateway.RunRequest{})
	if err == nil || !errors.Is(err, errPaidChatOutputBoundNotConverged) {
		t.Fatalf("err=%v, want non-converged output bound", err)
	}
}

func paidChatAssemblerPricingSnapshot(t *testing.T, contextWindow, maxOutput int64) string {
	t.Helper()
	model := officialmodel.ResolvedModel{
		Model: officialmodel.Model{
			CatalogVersion: "catalog-test", CatalogVendor: "openai", ModelID: "gpt-test",
			LifecycleStatus: officialmodel.LifecycleActive, ContextWindowTokens: contextWindow, MaxOutputTokens: maxOutput,
		},
		EffectivePrice: pricing.PriceBook{ModelID: "gpt-test", Rates: []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1},
		}},
		PriceSource: officialmodel.PriceSourceOfficial, PriceSourceURL: "https://example.test/pricing",
		PriceVerifiedAt: time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: "openai", RequestedModelID: "gpt-test",
		EffectiveMaxOutputTokens: int(maxOutput), MultiplierPPM: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type oscillatingPreparedChatTransport struct{}

func (oscillatingPreparedChatTransport) Capabilities() infraai.CapabilityMetadata {
	return infraai.CapabilityMetadata{}
}

func (oscillatingPreparedChatTransport) TestConnection(context.Context, infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (oscillatingPreparedChatTransport) StreamChat(context.Context, infraai.ChatInput, infraai.EventSink) (*infraai.ChatResult, error) {
	return nil, errors.New("not implemented")
}

func (oscillatingPreparedChatTransport) PrepareChat(_ context.Context, input infraai.ChatInput) ([]byte, error) {
	length := 98
	if input.EffectiveMaxOutputTokens == 100 || input.EffectiveMaxOutputTokens == 38 {
		length = 99
	}
	return []byte(strings.Repeat("x", length)), nil
}

func (oscillatingPreparedChatTransport) StreamPreparedChat(context.Context, infraai.PreparedChatRequest, infraai.EventSink) (*infraai.ChatResult, error) {
	return nil, errors.New("not implemented")
}

func stringInt(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var out [20]byte
	index := len(out)
	for value > 0 {
		index--
		out[index] = digits[value%10]
		value /= 10
	}
	return string(out[index:])
}
