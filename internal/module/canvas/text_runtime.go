package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	aibilling "admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type TextRuntimeService struct {
	repository    TextRepository
	billing       BillingRuleService
	secretbox     Secretbox
	engineFactory TextEngineFactory
	now           func() time.Time
}

type TextRuntimeDependencies struct {
	Repository    TextRepository
	Billing       BillingRuleService
	Secretbox     Secretbox
	EngineFactory TextEngineFactory
	Now           func() time.Time
}

func NewTextRuntimeService(deps TextRuntimeDependencies) *TextRuntimeService {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &TextRuntimeService{repository: deps.Repository, billing: deps.Billing, secretbox: deps.Secretbox, engineFactory: deps.EngineFactory, now: deps.Now}
}

func (s *TextRuntimeService) Generate(ctx context.Context, input TextGenerationInput) (*TextGenerationResponse, *apperror.Error) {
	input.Message = strings.TrimSpace(input.Message)
	input.ModelID = strings.TrimSpace(input.ModelID)
	if input.UserID <= 0 || input.AgentID <= 0 || input.Message == "" {
		return nil, apperror.BadRequestKey("canvas.ai.chat.request.invalid", nil, "文本生成参数错误")
	}
	agent, appErr := s.validTextAgent(ctx, input.AgentID)
	if appErr != nil {
		return nil, appErr
	}
	charge, appErr := s.charge(ctx, input, agent)
	if appErr != nil {
		return nil, appErr
	}
	engine, appErr := s.engine(ctx, agent)
	if appErr != nil {
		_ = s.refund(context.Background(), charge, appErr.Message)
		return nil, appErr
	}
	result, err := engine.StreamChat(ctx, infraai.ChatInput{
		AgentID: uint64(agent.AgentID),
		UserID:  uint64(input.UserID),
		UserKey: fmt.Sprintf("canvas:%d", input.UserID),
		Content: input.Message,
		Inputs:  textInputs(agent, agent.ModelID),
	}, discardSink{})
	if err != nil {
		_ = s.refund(context.Background(), charge, "Canvas文本生成失败")
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.provider_failed", nil, "Canvas文本生成失败", err)
	}
	answer := ""
	if result != nil {
		answer = strings.TrimSpace(result.Answer)
	}
	if answer == "" {
		_ = s.refund(context.Background(), charge, "Canvas文本生成结果为空")
		return nil, apperror.BadRequestKey("canvas.ai.chat.empty_result", nil, "Canvas文本生成结果为空")
	}
	if appErr := s.markSuccess(ctx, charge); appErr != nil {
		return nil, appErr
	}
	return &TextGenerationResponse{Content: answer}, nil
}

func (s *TextRuntimeService) validTextAgent(ctx context.Context, agentID int64) (*TextAgentRuntime, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.repository_missing", nil, "Canvas文本生成仓储未配置")
	}
	agent, err := s.repository.AgentForTextRuntime(ctx, agentID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.agent_query_failed", nil, "查询文本智能体失败", err)
	}
	if agent == nil || agent.AgentID <= 0 {
		return nil, apperror.NotFoundKey("canvas.ai.chat.agent_not_found", nil, "文本智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !supportsScene(agent.ScenesJSON, "chat") {
		return nil, apperror.BadRequestKey("canvas.ai.chat.agent_unavailable", nil, "该智能体不支持文本生成")
	}
	if strings.TrimSpace(agent.EngineAPIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("canvas.ai.chat.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	return agent, nil
}

func (s *TextRuntimeService) charge(ctx context.Context, input TextGenerationInput, agent *TextAgentRuntime) (*aibilling.ChargeResult, *apperror.Error) {
	if s == nil || s.billing == nil {
		return nil, apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
	}
	return s.billing.Charge(ctx, aibilling.ChargeInput{
		RequestNo:  fmt.Sprintf("CANVASTXT%d%d", input.UserID, s.now().UnixNano()),
		UserID:     input.UserID,
		Platform:   enum.PlatformCanvas,
		Scene:      aibilling.SceneCanvasTextGenerate,
		AgentID:    input.AgentID,
		ProviderID: agent.ProviderID,
		ModelID:    agent.ModelID,
		UnitCount:  1,
		Remark:     "Canvas文本生成",
	})
}

func (s *TextRuntimeService) engine(ctx context.Context, agent *TextAgentRuntime) (infraai.Engine, *apperror.Error) {
	if s == nil || s.secretbox == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.secretbox_missing", nil, "AI密钥服务未配置")
	}
	apiKey, err := s.secretbox.Decrypt(agent.EngineAPIKeyEnc)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.provider_key_decrypt_failed", nil, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequestKey("canvas.ai.chat.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.engine_missing", nil, "AI引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewEngine(ctx, TextEngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.engine_create_failed", nil, "创建AI引擎失败", err)
	}
	if engine == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.engine_missing", nil, "AI引擎未配置")
	}
	return engine, nil
}

func (s *TextRuntimeService) refund(ctx context.Context, charge *aibilling.ChargeResult, reason string) *apperror.Error {
	if charge == nil || charge.RecordID <= 0 || s == nil || s.billing == nil {
		return nil
	}
	return s.billing.Refund(ctx, aibilling.RefundInput{BillingRecordID: charge.RecordID, Reason: reason})
}

func (s *TextRuntimeService) markSuccess(ctx context.Context, charge *aibilling.ChargeResult) *apperror.Error {
	if charge == nil || charge.RecordID <= 0 || s == nil || s.billing == nil {
		return nil
	}
	successMarker, ok := s.billing.(interface {
		MarkSuccess(context.Context, int64) *apperror.Error
	})
	if !ok || successMarker == nil {
		return nil
	}
	return successMarker.MarkSuccess(ctx, charge.RecordID)
}

func textInputs(agent *TextAgentRuntime, modelID string) map[string]any {
	inputs := map[string]any{"model_id": modelID}
	if systemPrompt := strings.TrimSpace(agent.SystemPrompt); systemPrompt != "" {
		inputs["system_prompt"] = systemPrompt
	}
	return inputs
}

func supportsScene(raw string, want string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == want {
			return true
		}
	}
	return false
}

type discardSink struct{}

func (discardSink) Emit(ctx context.Context, event infraai.Event) error { return nil }
