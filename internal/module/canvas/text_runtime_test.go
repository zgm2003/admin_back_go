package canvas

import (
	"context"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	aibilling "admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type fakeTextRepository struct{ agent *TextAgentRuntime }

func (f *fakeTextRepository) AgentForTextRuntime(ctx context.Context, agentID int64) (*TextAgentRuntime, error) {
	return f.agent, nil
}

type fakeTextEngineFactory struct {
	engine infraai.Engine
	input  TextEngineConfig
}

func (f *fakeTextEngineFactory) NewEngine(ctx context.Context, input TextEngineConfig) (infraai.Engine, error) {
	f.input = input
	return f.engine, nil
}

type fakeTextEngine struct {
	input  infraai.ChatInput
	err    error
	answer string
}

func (f *fakeTextEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}
func (f *fakeTextEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &infraai.ChatResult{Answer: f.answer}, nil
}

type fakeTextBilling struct {
	fakeSettingsBilling
	markSuccessID int64
}

func (f *fakeTextBilling) MarkSuccess(ctx context.Context, id int64) *apperror.Error {
	f.markSuccessID = id
	return nil
}

func TestTextRuntimeChargesCanvasTextBeforeProviderAndMarksSuccess(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	engine := &fakeTextEngine{answer: "你好"}
	factory := &fakeTextEngineFactory{engine: engine}
	billing := &fakeTextBilling{fakeSettingsBilling: fakeSettingsBilling{chargeResult: &aibilling.ChargeResult{RecordID: 88}}}
	svc := NewTextRuntimeService(TextRuntimeDependencies{Repository: &fakeTextRepository{agent: &TextAgentRuntime{AgentID: 8, ProviderID: 9, ModelID: "gpt-4.1-mini", ScenesJSON: `["chat"]`, EngineType: string(infraai.EngineTypeOpenAI), EngineBaseURL: "https://api.openai.test/v1", EngineAPIKeyEnc: cipher, AgentStatus: enum.CommonYes, EngineStatus: enum.CommonYes}}, Billing: billing, Secretbox: box, EngineFactory: factory})

	result, appErr := svc.Generate(context.Background(), TextGenerationInput{UserID: 7, AgentID: 8, Message: "hi"})

	if appErr != nil {
		t.Fatalf("Generate error=%#v", appErr)
	}
	if result.Content != "你好" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if billing.chargeInput.Platform != "canvas" || billing.chargeInput.Scene != aibilling.SceneCanvasTextGenerate || billing.chargeInput.UserID != 7 || billing.chargeInput.UnitCount != 1 {
		t.Fatalf("charge input mismatch: %#v", billing.chargeInput)
	}
	if factory.input.APIKey != "provider-key" || engine.input.Content != "hi" || engine.input.Inputs["model_id"] != "gpt-4.1-mini" {
		t.Fatalf("engine input mismatch factory=%#v chat=%#v", factory.input, engine.input)
	}
	if billing.markSuccessID != 88 || len(billing.refundInputs) != 0 {
		t.Fatalf("billing terminal mismatch success=%d refunds=%#v", billing.markSuccessID, billing.refundInputs)
	}
}

func TestTextRuntimeUsesAgentModelInsteadOfClientSuppliedModel(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	engine := &fakeTextEngine{answer: "你好"}
	svc := NewTextRuntimeService(TextRuntimeDependencies{
		Repository:    &fakeTextRepository{agent: &TextAgentRuntime{AgentID: 8, ProviderID: 9, ModelID: "gpt-4.1-mini", ScenesJSON: `["chat"]`, EngineType: string(infraai.EngineTypeOpenAI), EngineAPIKeyEnc: cipher, AgentStatus: enum.CommonYes, EngineStatus: enum.CommonYes}},
		Billing:       &fakeTextBilling{fakeSettingsBilling: fakeSettingsBilling{chargeResult: &aibilling.ChargeResult{RecordID: 88}}},
		Secretbox:     box,
		EngineFactory: &fakeTextEngineFactory{engine: engine},
	})

	_, appErr := svc.Generate(context.Background(), TextGenerationInput{UserID: 7, AgentID: 8, ModelID: "client-invented-model", Message: "hi"})

	if appErr != nil {
		t.Fatalf("Generate error=%#v", appErr)
	}
	if engine.input.Inputs["model_id"] != "gpt-4.1-mini" {
		t.Fatalf("client model override must be ignored; engine input=%#v", engine.input.Inputs)
	}
}

func TestTextRuntimeRefundsOnceWhenProviderFails(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	billing := &fakeTextBilling{fakeSettingsBilling: fakeSettingsBilling{chargeResult: &aibilling.ChargeResult{RecordID: 88}}}
	svc := NewTextRuntimeService(TextRuntimeDependencies{Repository: &fakeTextRepository{agent: &TextAgentRuntime{AgentID: 8, ProviderID: 9, ModelID: "gpt-4.1-mini", ScenesJSON: `["chat"]`, EngineType: string(infraai.EngineTypeOpenAI), EngineAPIKeyEnc: cipher, AgentStatus: enum.CommonYes, EngineStatus: enum.CommonYes}}, Billing: billing, Secretbox: box, EngineFactory: &fakeTextEngineFactory{engine: &fakeTextEngine{err: errors.New("provider down")}}})

	_, appErr := svc.Generate(context.Background(), TextGenerationInput{UserID: 7, AgentID: 8, Message: "hi"})

	if appErr == nil || appErr.MessageID != "canvas.ai.chat.provider_failed" {
		t.Fatalf("expected provider failure, got %#v", appErr)
	}
	if len(billing.refundInputs) != 1 || billing.refundInputs[0].BillingRecordID != 88 {
		t.Fatalf("expected one refund, got %#v", billing.refundInputs)
	}
}
