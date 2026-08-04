package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/openaicompat"
	"admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPaidChatAssemblerConvergesPreparedRequestAndSafeOutputBound(t *testing.T) {
	snapshot := paidChatAssemblerPricingSnapshot(t, 300, 100)
	assembler := paidChatAssembler{
		transport: openaicompat.New(openaicompat.Config{}),
		input: infraai.ChatInput{
			ModelID: "forged",
			Messages: []infraai.Message{{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{
				Kind: infraai.ContentPartText, Text: "hello",
			}}}},
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
	if !strings.Contains(string(call.RequestBody), `"model":"gpt-test"`) || strings.Contains(string(call.RequestBody), `"model":"forged"`) ||
		!strings.Contains(string(call.RequestBody), `"max_tokens":`+stringInt(int(wantOutputBound))) {
		t.Fatalf("prepared request did not freeze the converged system bound: %s", call.RequestBody)
	}
}

func TestPaidChatAssemblerConvergesResponsesPreparedRequestAndSafeOutputBound(t *testing.T) {
	const contextWindow = int64(1200)
	const maxOutput = int64(100)
	assembler := paidChatAssembler{
		transport: openaicompat.New(openaicompat.Config{APIProtocol: infraai.APIProtocolResponses}),
		input: infraai.ChatInput{
			ModelID: "forged",
			Messages: []infraai.Message{{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{
				Kind: infraai.ContentPartText, Text: "hello",
			}}}},
		},
	}

	call, err := assembler.AssembleAndQuote(context.Background(), aigateway.RunSnapshot{
		RunID: 3, ModelID: "gpt-test", PricingSnapshotJSON: paidChatAssemblerPricingSnapshot(t, contextWindow, maxOutput),
	}, aigateway.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if call.Quote.PreparedRequestSchema != infraai.PreparedChatSchemaResponsesInlineV1 ||
		call.Quote.InputUpperBoundStrategy != infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1 {
		t.Fatalf("quote=%+v", call.Quote)
	}
	inputBound, err := infraai.SafeInputUpperBoundFromRequest(call.RequestBody)
	if err != nil {
		t.Fatal(err)
	}
	wantOutputBound := contextWindow - inputBound
	if wantOutputBound > maxOutput {
		wantOutputBound = maxOutput
	}
	if call.Quote.EffectiveMaxOutputTokens != int(wantOutputBound) {
		t.Fatalf("effective output bound=%d, want %d", call.Quote.EffectiveMaxOutputTokens, wantOutputBound)
	}
	envelope, err := infraai.ParsePreparedChatInlineEnvelope(call.RequestBody)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envelope.Request), `"model":"gpt-test"`) || strings.Contains(string(envelope.Request), `"model":"forged"`) ||
		!strings.Contains(string(envelope.Request), `"max_output_tokens":`+stringInt(int(wantOutputBound))) {
		t.Fatalf("prepared Responses request did not freeze the converged system bound: %s", envelope.Request)
	}
}

func TestPaidChatAssemblerFailsClosedWhenBoundDoesNotConverge(t *testing.T) {
	assembler := paidChatAssembler{
		transport: oscillatingPreparedChatTransport{},
		input: infraai.ChatInput{ModelID: "forged", Messages: []infraai.Message{{
			Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "hello"}},
		}}},
	}
	_, err := assembler.AssembleAndQuote(context.Background(), aigateway.RunSnapshot{
		RunID: 2, ModelID: "gpt-test", PricingSnapshotJSON: paidChatAssemblerPricingSnapshot(t, 200, 100),
	}, aigateway.RunRequest{})
	if err == nil || !errors.Is(err, errPaidChatOutputBoundNotConverged) {
		t.Fatalf("err=%v, want non-converged output bound", err)
	}
}

func TestClonePaidChatInputDeepCopiesTypedRequest(t *testing.T) {
	temperature := 0.25
	input := infraai.ChatInput{
		ModelID: "gpt-original",
		Messages: []infraai.Message{
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "hello"}}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{
				Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{Kind: infraai.AttachmentImage, URL: "https://example.test/a.png"},
			}}},
		},
		Temperature: &temperature,
		Tools: []infraai.ToolDefinition{{Name: "lookup", Parameters: map[string]any{
			"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}},
		}}},
		ToolCalls:    []infraai.ToolCall{{ID: "call-1", Name: "lookup", Arguments: `{"id":1}`}},
		ToolOutputs:  []infraai.ToolOutput{{CallID: "call-1", Name: "lookup", Output: "one"}},
		Continuation: &infraai.ChatContinuation{Protocol: infraai.APIProtocolResponses, Items: json.RawMessage(`[{"type":"function_call"}]`)},
	}

	cloned := clonePaidChatInput(input)
	cloned.Messages[0].Parts[0].Text = "changed"
	cloned.Messages[1].Parts[0].Attachment.URL = "https://example.test/changed.png"
	*cloned.Temperature = 0.9
	cloned.Tools[0].Parameters["type"] = "changed"
	cloned.Tools[0].Parameters["properties"].(map[string]any)["id"].(map[string]any)["type"] = "string"
	cloned.ToolCalls[0].Name = "changed"
	cloned.ToolOutputs[0].Output = "changed"
	cloned.Continuation.Items[0] = '{'

	if input.Messages[0].Parts[0].Text != "hello" || input.Messages[1].Parts[0].Attachment.URL != "https://example.test/a.png" ||
		*input.Temperature != 0.25 || input.Tools[0].Parameters["type"] != "object" ||
		input.Tools[0].Parameters["properties"].(map[string]any)["id"].(map[string]any)["type"] != "integer" ||
		input.ToolCalls[0].Name != "lookup" || input.ToolOutputs[0].Output != "one" || input.Continuation.Items[0] != '[' {
		t.Fatalf("clone mutated original input: %#v", input)
	}
}

func TestGatewayAttemptFromRowPreservesContextPlanEvidence(t *testing.T) {
	request := `{"model":"gpt-test"}`
	requestHash := sha256.Sum256([]byte(request))
	planHash := sha256.Sum256([]byte("ready plan"))
	planID := uint64(91)
	quote := aigateway.QuoteEvidence{PricingVersion: "v1", EffectiveMaxOutputTokens: 10, CurrentCallMaxUnits: 25, TargetHoldUnits: 25, UpperBoundItems: []billing.UsageItem{{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}}}
	quote.PreparedRequestSHA256 = requestHash
	quoteJSON, err := json.Marshal(quote)
	if err != nil {
		t.Fatal(err)
	}
	row := replycommand.Attempt{
		ID: 71, RunID: 44, AttemptNo: 1, IdempotencyKey: strings.Repeat("a", 64),
		PreparedRequestJSON: request, PreparedRequestSHA256: requestHash[:], QuoteJSON: string(quoteJSON),
		ContextPlanID: &planID, ContextPlanSHA256: planHash[:],
	}

	attempt, err := gatewayAttemptFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.ContextPlan == nil || attempt.ContextPlan.ID != planID || attempt.ContextPlan.SHA256 != planHash {
		t.Fatalf("attempt=%+v", attempt)
	}
}

func TestGatewayAttemptFromRowRejectsPartialContextPlanEvidence(t *testing.T) {
	request := `{"model":"gpt-test"}`
	requestHash := sha256.Sum256([]byte(request))
	planID := uint64(91)
	quote := aigateway.QuoteEvidence{PricingVersion: "v1", EffectiveMaxOutputTokens: 10, CurrentCallMaxUnits: 25, TargetHoldUnits: 25, UpperBoundItems: []billing.UsageItem{{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 1}}}
	quote.PreparedRequestSHA256 = requestHash
	quoteJSON, err := json.Marshal(quote)
	if err != nil {
		t.Fatal(err)
	}
	base := replycommand.Attempt{
		ID: 71, RunID: 44, AttemptNo: 1, IdempotencyKey: strings.Repeat("a", 64),
		PreparedRequestJSON: request, PreparedRequestSHA256: requestHash[:], QuoteJSON: string(quoteJSON),
	}
	for _, row := range []replycommand.Attempt{
		func() replycommand.Attempt { row := base; row.ContextPlanID = &planID; return row }(),
		func() replycommand.Attempt {
			row := base
			row.ContextPlanSHA256 = make([]byte, sha256.Size)
			return row
		}(),
	} {
		if _, err := gatewayAttemptFromRow(row); err == nil {
			t.Fatalf("partial context plan evidence was accepted: %+v", row)
		}
	}
}

func TestETagMismatchBeforeDispatchIsPermanentPreDispatchFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "etag mismatch", err: cos.ErrObjectVersionChanged},
		{name: "object unavailable", err: cos.ErrObjectUnavailable},
		{name: "untrusted object key", err: cos.ErrUntrustedObjectKey},
		{name: "invalid object metadata", err: cos.ErrInvalidObjectMetadata},
		{name: "invalid provider config", err: infraai.ErrInvalidConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := fmt.Errorf("native file preflight failed: %w", test.err)
			if !isPermanentPreDispatchError(err) {
				t.Fatalf("isPermanentPreDispatchError(%v) = false", err)
			}
		})
	}
}

func TestMustFinalizePreDispatchErrorAtCommandAttemptBoundary(t *testing.T) {
	retryable := apperror.New("dependency.storage", apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, "", nil, "storage unavailable")
	if mustFinalizePreDispatchError(aichat.PaidChatAttemptInput{CommandAttempt: 1, CommandMaxAttempts: 3}, retryable) {
		t.Fatal("retryable pre-dispatch error finalized before the command attempt boundary")
	}
	if !mustFinalizePreDispatchError(aichat.PaidChatAttemptInput{CommandAttempt: 3, CommandMaxAttempts: 3}, retryable) {
		t.Fatal("retryable pre-dispatch error remained open after the command attempt boundary")
	}
	if !mustFinalizePreDispatchError(aichat.PaidChatAttemptInput{CommandAttempt: 1, CommandMaxAttempts: 3}, infraai.ErrInvalidConfig) {
		t.Fatal("permanent pre-dispatch error remained retryable")
	}
}

func TestGatewayOwnerGuardRejectsDeletedConversationAtDispatchBoundary(t *testing.T) {
	db, mock, closeDB := newFinalizerMockDB(t)
	defer closeDB()
	now := time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT .* FROM ` + "`ai_reply_commands`" + `.*EXISTS \(SELECT 1 FROM ai_conversations c WHERE c\.id = ai_reply_commands\.conversation_id AND c\.user_id = ai_reply_commands\.user_id AND c\.is_del = \?\).*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err := (gormGatewayOwnerGuard{commandID: 41, owner: "worker-a", token: 7, now: func() time.Time { return now }}).
		EnsureRunnable(context.Background(), gormBillingTransaction{db: db}, 51)
	if !errors.Is(err, replycommand.ErrLeaseLost) {
		t.Fatalf("deleted conversation owner guard error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
	return []byte(`{"padding":"` + strings.Repeat("x", length-len(`{"padding":""}`)) + `"}`), nil
}

func (oscillatingPreparedChatTransport) PreflightPreparedChat(context.Context, []byte) (*infraai.FileInputMetrics, error) {
	return nil, nil
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
