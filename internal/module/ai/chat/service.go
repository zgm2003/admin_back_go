package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/secretbox"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const defaultTimeoutLimit = 100
const defaultRunStaleTimeout = 15 * time.Minute
const historyLimit = 20
const maxHistoryLimit = 50
const canvasTextGenerateScene = "canvas_text_generate"

type Dependencies struct {
	Repository       Repository
	Publisher        infrarealtime.Publisher
	EngineFactory    EngineFactory
	Secretbox        secretbox.Box
	ToolRuntime      ToolRuntime
	KnowledgeRuntime KnowledgeRuntime
	RunRecorder      RunRecorder
	TextTasks        TextTaskStore
	RunStaleTimeout  time.Duration
	Now              func() time.Time
}

type Service struct {
	repository       Repository
	publisher        infrarealtime.Publisher
	engineFactory    EngineFactory
	secretbox        secretbox.Box
	toolRuntime      ToolRuntime
	knowledgeRuntime KnowledgeRuntime
	runRecorder      RunRecorder
	textTasks        TextTaskStore
	runStaleTimeout  time.Duration
	now              func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	runStaleTimeout := deps.RunStaleTimeout
	if runStaleTimeout <= 0 {
		runStaleTimeout = defaultRunStaleTimeout
	}
	return &Service{repository: deps.Repository, publisher: deps.Publisher, engineFactory: deps.EngineFactory, secretbox: deps.Secretbox, toolRuntime: deps.ToolRuntime, knowledgeRuntime: deps.KnowledgeRuntime, runRecorder: deps.RunRecorder, textTasks: deps.TextTasks, runStaleTimeout: runStaleTimeout, now: now}
}

func (s *Service) ExecuteConversationReply(ctx context.Context, input ConversationReplyInput) (*ConversationReplyResult, error) {
	if input.ConversationID <= 0 || input.UserID <= 0 || input.AgentID <= 0 || input.UserMessageID <= 0 || strings.TrimSpace(input.RequestID) == "" {
		return nil, apperror.BadRequest("AI对话回复任务参数错误")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	conversation, err := repo.ConversationForReply(ctx, input.ConversationID, input.UserID)
	if err != nil {
		return nil, err
	}
	if conversation == nil {
		return nil, apperror.NotFound("AI会话不存在")
	}
	if int64(conversation.AgentID) != input.AgentID {
		return nil, apperror.BadRequest("会话智能体不匹配")
	}
	agent, err := repo.AgentForRuntime(ctx, uint64(input.AgentID))
	if err != nil {
		return nil, err
	}
	if agent == nil || agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !agentSupportsChat(agent.ScenesJSON) {
		msg := "该智能体不支持对话场景"
		_ = s.publishFailed(ctx, input, msg)
		return nil, apperror.BadRequest(msg)
	}
	if err := s.publishStart(ctx, input); err != nil {
		return nil, err
	}
	engine, appErr := s.engineForAgent(ctx, *agent)
	if appErr != nil {
		_ = s.publishFailed(ctx, input, appErr.Message)
		return nil, appErr
	}
	history, err := repo.LatestMessages(ctx, input.ConversationID, maxHistoryLimit+1)
	if err != nil {
		msg := "读取AI消息历史失败"
		_ = s.publishFailed(ctx, input, msg)
		return nil, err
	}
	userMessage, ok := userMessageForID(history, input.UserMessageID)
	if !ok {
		msg := "用户消息不存在"
		_ = s.publishFailed(ctx, input, msg)
		appErr := apperror.BadRequest(msg)
		return nil, appErr
	}
	userContent := userMessage.Content
	inputSnapshot, appErr := chatRunInputSnapshot(userMessage)
	if appErr != nil {
		msg := appErr.Message
		_ = s.publishFailed(ctx, input, msg)
		return nil, appErr
	}
	if s.runRecorder == nil {
		msg := "AI运行记录服务未配置"
		_ = s.publishFailed(ctx, input, msg)
		return nil, apperror.Internal(msg)
	}
	startedAt := s.now()
	conversationID := input.ConversationID
	userMessageID := input.UserMessageID
	runID, err := s.runRecorder.Start(ctx, airun.StartInput{
		Platform:         enum.PlatformAdmin,
		ConversationID:   &conversationID,
		UserMessageID:    &userMessageID,
		RequestID:        input.RequestID,
		UserID:           input.UserID,
		AgentID:          input.AgentID,
		ProviderID:       int64(agent.ProviderID),
		ModelID:          agent.ModelID,
		ModelDisplayName: agent.ModelDisplayName,
		InputSnapshot:    inputSnapshot,
		StartedAt:        startedAt,
	})
	if err != nil {
		_ = s.publishFailed(ctx, input, "创建AI运行记录失败")
		return nil, err
	}
	finishRun := func(status string, msg string, cause error) {
		finishedAt := s.now()
		finishInput := airun.FailInput{RunID: runID, Message: msg, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)}
		switch status {
		case enum.AIRunStatusCanceled:
			_ = s.runRecorder.Cancel(context.Background(), airun.CancelInput(finishInput))
		case enum.AIRunStatusTimeout:
			_ = s.runRecorder.Timeout(context.Background(), airun.TimeoutInput(finishInput))
		default:
			_ = s.runRecorder.Fail(context.Background(), finishInput)
		}
	}
	if s.knowledgeRuntime != nil {
		knowledge, knowledgeErr := s.knowledgeRuntime.RetrieveForRun(ctx, KnowledgeRuntimeInput{
			RunID:          uint64(runID),
			AgentID:        uint64(input.AgentID),
			ConversationID: input.ConversationID,
			UserMessageID:  input.UserMessageID,
			Query:          userContent,
			StartedAt:      startedAt,
		})
		if knowledgeErr == nil && knowledge != nil && strings.TrimSpace(knowledge.Context) != "" {
			userContent = strings.TrimSpace(knowledge.Context) + "\n\n用户问题：\n" + userContent
		}
	}
	sink := &conversationEventSink{service: s, input: input}
	chatInput := infraai.ChatInput{
		AgentID: uint64(input.AgentID),
		RunID:   uint64(runID),
		UserID:  uint64(input.UserID),
		UserKey: userKey(input.UserID),
		Content: userContent,
		Inputs:  chatInputs(*agent, history, input.UserMessageID),
	}
	runtimeTools, appErr := s.runtimeTools(ctx, uint64(input.AgentID))
	if appErr != nil {
		_ = s.publishFailed(ctx, input, appErr.Message)
		finishRun(enum.AIRunStatusFailed, appErr.Message, appErr)
		return nil, appErr
	}
	chatInput.Tools = toolDefinitions(runtimeTools)
	result, err := engine.StreamChat(ctx, chatInput, sink)
	if err != nil {
		msg := err.Error()
		_ = s.publishFailed(ctx, input, msg)
		finishRun(statusFromError(ctx, err), msg, err)
		return nil, err
	}
	if toolCalls := resultToolCalls(result); len(toolCalls) > 0 {
		if appErr := validateRunUsageStatus(result); appErr != nil {
			msg := appErr.Message
			_ = s.publishFailed(ctx, input, msg)
			finishRun(enum.AIRunStatusFailed, msg, appErr)
			return nil, appErr
		}
		firstUsage := resultTokens(result)
		outputs, toolErr := s.executeToolCalls(ctx, uint64(runID), runtimeTools, toolCalls)
		if toolErr != nil {
			msg := toolErr.Error()
			_ = s.publishFailed(ctx, input, msg)
			finishRun(enum.AIRunStatusFailed, msg, toolErr)
			return nil, toolErr
		}
		chatInput.ToolCalls = toolCalls
		chatInput.ToolOutputs = outputs
		result, err = engine.StreamChat(ctx, chatInput, sink)
		if err != nil {
			msg := err.Error()
			_ = s.publishFailed(ctx, input, msg)
			finishRun(statusFromError(ctx, err), msg, err)
			return nil, err
		}
		if appErr := validateRunUsageStatus(result); appErr != nil {
			msg := appErr.Message
			_ = s.publishFailed(ctx, input, msg)
			finishRun(enum.AIRunStatusFailed, msg, appErr)
			return nil, appErr
		}
		addTokenUsage(result, firstUsage)
		if len(resultToolCalls(result)) > 0 {
			msg := "工具调用轮次超过MVP限制"
			appErr := apperror.BadRequest(msg)
			_ = s.publishFailed(ctx, input, msg)
			finishRun(enum.AIRunStatusFailed, msg, appErr)
			return nil, appErr
		}
	}
	answer := ""
	if result != nil {
		answer = result.Answer
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "AI没有返回内容"
		if err := s.publishDelta(ctx, input, answer); err != nil {
			return nil, err
		}
	}
	if appErr := validateRunUsageStatus(result); appErr != nil {
		msg := appErr.Message
		_ = s.publishFailed(ctx, input, msg)
		finishRun(enum.AIRunStatusFailed, msg, appErr)
		return nil, appErr
	}
	assistantID, err := repo.InsertAssistantMessage(ctx, AssistantMessageRecord{ConversationID: input.ConversationID, Content: answer, Now: s.now()})
	if err != nil {
		msg := "保存AI助手消息失败"
		_ = s.publishFailed(ctx, input, msg)
		finishRun(enum.AIRunStatusFailed, msg, err)
		return nil, err
	}
	finishedAt := s.now()
	assistantMessageID := assistantID
	tokens := resultTokens(result)
	if err := s.runRecorder.Complete(context.Background(), airun.CompleteInput{RunID: runID, AssistantMessageID: &assistantMessageID, PromptTokens: tokens.Prompt, CompletionTokens: tokens.Completion, TotalTokens: tokens.Total, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)}); err != nil {
		msg := "更新AI运行记录失败"
		_ = s.publishFailed(ctx, input, msg)
		return nil, err
	}
	if err := s.publishCompleted(ctx, input, assistantID); err != nil {
		return nil, err
	}
	return &ConversationReplyResult{ConversationID: input.ConversationID, AssistantMessageID: assistantID}, nil
}

func (s *Service) CanvasCompletion(ctx context.Context, input CanvasCompletionInput) (*CanvasCompletionResponse, *apperror.Error) {
	input.Message = strings.TrimSpace(input.Message)
	input.ModelID = strings.TrimSpace(input.ModelID)
	if input.UserID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.AgentID <= 0 || input.Message == "" {
		return nil, apperror.BadRequestKey("canvas.ai.chat.request.invalid", nil, "文本生成参数错误")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, apperror.InternalKey("canvas.ai.chat.repository_missing", nil, "Canvas文本生成仓储未配置")
	}
	agent, err := repo.AgentForRuntime(ctx, uint64(input.AgentID))
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.agent_query_failed", nil, "查询文本智能体失败", err)
	}
	if agent == nil || agent.AgentID == 0 {
		return nil, apperror.NotFoundKey("canvas.ai.chat.agent_not_found", nil, "文本智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !agentSupportsCanvasText(agent.ScenesJSON) {
		return nil, apperror.BadRequestKey("canvas.ai.chat.agent_unavailable", nil, "该智能体不支持文本生成")
	}
	engine, appErr := s.canvasCompletionEngine(ctx, *agent)
	if appErr != nil {
		return nil, appErr
	}
	if s.textTasks == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.text_task_store_missing", nil, "Canvas文本任务仓储未配置")
	}
	if s.runRecorder == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.run_recorder_missing", nil, "Canvas文本运行记录服务未配置")
	}
	startedAt := s.now()
	textTaskID, err := s.textTasks.Create(ctx, aitext.CreateInput{
		Platform:   enum.PlatformCanvas,
		UserID:     input.UserID,
		AgentID:    agent.AgentID,
		ProviderID: agent.ProviderID,
		ModelID:    agent.ModelID,
		Prompt:     input.Message,
		Status:     aitext.StatusRunning,
		StartedAt:  startedAt,
		CreatedAt:  startedAt,
		UpdatedAt:  startedAt,
	})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.text_task_failed", nil, "创建Canvas文本任务失败", err)
	}
	runID, err := s.runRecorder.Start(ctx, airun.StartInput{
		Platform:         enum.PlatformCanvas,
		RequestID:        "ai_text_task_" + strconv.FormatUint(textTaskID, 10),
		UserID:           input.UserID,
		AgentID:          int64(agent.AgentID),
		ProviderID:       int64(agent.ProviderID),
		ModelID:          agent.ModelID,
		ModelDisplayName: agent.ModelDisplayName,
		InputSnapshot:    input.Message,
		StartedAt:        startedAt,
	})
	if err != nil {
		finishedAt := s.now()
		_ = s.textTasks.Fail(context.Background(), aitext.FailInput{ID: textTaskID, ErrorMessage: "创建Canvas文本运行记录失败", FinishedAt: finishedAt, ElapsedMS: durationMS(startedAt, finishedAt)})
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.run_start_failed", nil, "创建Canvas文本运行记录失败", err)
	}
	result, err := engine.StreamChat(ctx, infraai.ChatInput{
		AgentID: agent.AgentID,
		UserID:  uint64(input.UserID),
		UserKey: canvasUserKey(input.UserID),
		Content: input.Message,
		Inputs:  canvasCompletionInputs(*agent),
	}, discardEventSink{})
	if err != nil {
		finishedAt := s.now()
		_ = s.textTasks.Fail(context.Background(), aitext.FailInput{ID: textTaskID, ErrorMessage: "Canvas文本生成失败", FinishedAt: finishedAt, ElapsedMS: durationMS(startedAt, finishedAt)})
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: "Canvas文本生成失败", FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.provider_failed", nil, "Canvas文本生成失败", err)
	}
	answer := ""
	if result != nil {
		answer = strings.TrimSpace(result.Answer)
	}
	if answer == "" {
		finishedAt := s.now()
		_ = s.textTasks.Fail(context.Background(), aitext.FailInput{ID: textTaskID, ErrorMessage: "Canvas文本生成结果为空", FinishedAt: finishedAt, ElapsedMS: durationMS(startedAt, finishedAt)})
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: "Canvas文本生成结果为空", FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, apperror.BadRequestKey("canvas.ai.chat.empty_result", nil, "Canvas文本生成结果为空")
	}
	if appErr := validateRunUsageStatus(result); appErr != nil {
		finishedAt := s.now()
		_ = s.textTasks.Fail(context.Background(), aitext.FailInput{ID: textTaskID, ErrorMessage: appErr.Message, FinishedAt: finishedAt, ElapsedMS: durationMS(startedAt, finishedAt)})
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: appErr.Message, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, appErr
	}
	tokens := resultTokens(result)
	finishedAt := s.now()
	if err := s.textTasks.Complete(ctx, aitext.CompleteInput{ID: textTaskID, Answer: answer, FinishedAt: finishedAt, ElapsedMS: durationMS(startedAt, finishedAt)}); err != nil {
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: "更新Canvas文本任务失败", FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.text_task_complete_failed", nil, "更新Canvas文本任务失败", err)
	}
	if err := s.runRecorder.Complete(context.Background(), airun.CompleteInput{RunID: runID, PromptTokens: tokens.Prompt, CompletionTokens: tokens.Completion, TotalTokens: tokens.Total, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)}); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.run_complete_failed", nil, "更新Canvas文本运行记录失败", err)
	}
	return &CanvasCompletionResponse{ID: fmt.Sprintf("canvas-chat-%d", s.now().UnixNano()), Object: "chat.completion", Content: answer}, nil
}

type tokenResult struct {
	Prompt, Completion, Total int
	UsageStatus               string
}

func resultTokens(result *infraai.ChatResult) tokenResult {
	if result == nil {
		return tokenResult{}
	}
	return tokenResult{Prompt: result.PromptTokens, Completion: result.CompletionTokens, Total: result.TotalTokens, UsageStatus: result.UsageStatus}
}

func addTokenUsage(result *infraai.ChatResult, usage tokenResult) {
	if result == nil {
		return
	}
	result.PromptTokens += usage.Prompt
	result.CompletionTokens += usage.Completion
	result.TotalTokens += usage.Total
	if result.UsageStatus == infraai.UsageStatusReported && usage.UsageStatus == infraai.UsageStatusReported {
		result.UsageStatus = infraai.UsageStatusReported
		return
	}
	result.UsageStatus = infraai.UsageStatusUnavailable
}

func validateRunUsageStatus(result *infraai.ChatResult) *apperror.Error {
	if result == nil {
		return apperror.InternalKey("ai.run.usage_status_missing", nil, "AI供应商用量状态缺失")
	}
	switch result.UsageStatus {
	case infraai.UsageStatusReported, infraai.UsageStatusUnavailable:
		return nil
	default:
		return apperror.InternalKey("ai.run.usage_status_missing", nil, "AI供应商用量状态缺失")
	}
}

func durationMS(startedAt time.Time, finishedAt time.Time) uint {
	if startedAt.IsZero() || finishedAt.Before(startedAt) {
		return 0
	}
	return uint(finishedAt.Sub(startedAt).Milliseconds())
}

func statusFromError(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return enum.AIRunStatusCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return enum.AIRunStatusTimeout
	}
	return enum.AIRunStatusFailed
}
func (s *Service) TimeoutRuns(ctx context.Context, input RunTimeoutInput) (*RunTimeoutResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultTimeoutLimit
	}
	staleTimeout := input.StaleTimeout
	if staleTimeout <= 0 {
		staleTimeout = s.runStaleTimeout
	}
	if staleTimeout <= 0 {
		staleTimeout = defaultRunStaleTimeout
	}
	staleBefore := s.now().Add(-staleTimeout)
	count, err := repo.TimeoutRuns(ctx, limit, staleBefore, "AI运行残留超时")
	if err != nil {
		return nil, err
	}
	return &RunTimeoutResult{Failed: count}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI对话仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) engineForAgent(ctx context.Context, agent AgentEngineConfig) (infraai.Engine, *apperror.Error) {
	if agent.AgentID == 0 || agent.ProviderID == 0 {
		return nil, apperror.BadRequest("AI智能体或供应商未配置")
	}
	apiKeyEnc := strings.TrimSpace(agent.EngineAPIKeyEnc)
	if apiKeyEnc == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.Internal("AI引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewEngine(ctx, EngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "创建AI引擎失败", err)
	}
	return engine, nil
}

func (s *Service) canvasCompletionEngine(ctx context.Context, agent AgentEngineConfig) (infraai.Engine, *apperror.Error) {
	if agent.AgentID == 0 || agent.ProviderID == 0 {
		return nil, apperror.BadRequestKey("canvas.ai.chat.agent_unavailable", nil, "该智能体不支持文本生成")
	}
	apiKeyEnc := strings.TrimSpace(agent.EngineAPIKeyEnc)
	if apiKeyEnc == "" {
		return nil, apperror.BadRequestKey("canvas.ai.chat.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.provider_key_decrypt_failed", nil, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequestKey("canvas.ai.chat.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.engine_missing", nil, "AI引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewEngine(ctx, EngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.chat.engine_create_failed", nil, "创建AI引擎失败", err)
	}
	if engine == nil {
		return nil, apperror.InternalKey("canvas.ai.chat.engine_missing", nil, "AI引擎未配置")
	}
	return engine, nil
}

func (s *Service) publishStart(ctx context.Context, input ConversationReplyInput) error {
	event, err := BuildStartEvent(StartPayload{ConversationID: input.ConversationID, RequestID: input.RequestID, UserMessageID: input.UserMessageID, AgentID: input.AgentID})
	if err != nil {
		return err
	}
	return s.publish(ctx, input.UserID, event)
}

func (s *Service) publishDelta(ctx context.Context, input ConversationReplyInput, delta string) error {
	if strings.TrimSpace(delta) == "" {
		return nil
	}
	event, err := BuildDeltaEvent(DeltaPayload{ConversationID: input.ConversationID, RequestID: input.RequestID, Delta: delta})
	if err != nil {
		return err
	}
	return s.publish(ctx, input.UserID, event)
}

func (s *Service) publishCompleted(ctx context.Context, input ConversationReplyInput, assistantMessageID int64) error {
	event, err := BuildCompletedEvent(CompletedPayload{ConversationID: input.ConversationID, RequestID: input.RequestID, AssistantMessageID: assistantMessageID})
	if err != nil {
		return err
	}
	return s.publish(ctx, input.UserID, event)
}

func (s *Service) publishFailed(ctx context.Context, input ConversationReplyInput, msg string) error {
	event, err := BuildFailedEvent(FailedPayload{ConversationID: input.ConversationID, RequestID: input.RequestID, Msg: msg})
	if err != nil {
		return err
	}
	return s.publish(ctx, input.UserID, event)
}

func (s *Service) publish(ctx context.Context, userID int64, event EnvelopeEvent) error {
	if s.publisher == nil {
		return nil
	}
	return s.publisher.Publish(ctx, infrarealtime.Publication{Platform: enum.PlatformAdmin, UserID: userID, Envelope: event.Envelope})
}

type conversationEventSink struct {
	service *Service
	input   ConversationReplyInput
}

func (s *conversationEventSink) Emit(ctx context.Context, event infraai.Event) error {
	if s == nil || s.service == nil {
		return nil
	}
	if event.Type == "delta" {
		delta := event.DeltaText
		if delta == "" && event.Payload != nil {
			if value, ok := event.Payload["delta"].(string); ok {
				delta = value
			}
		}
		return s.service.publishDelta(ctx, s.input, delta)
	}
	if event.Type == "failed" {
		msg := "AI回复失败"
		if event.Payload != nil {
			if value, ok := event.Payload["message"].(string); ok && strings.TrimSpace(value) != "" {
				msg = value
			}
		}
		return s.service.publishFailed(ctx, s.input, msg)
	}
	return nil
}

func canvasCompletionInputs(agent AgentEngineConfig) map[string]any {
	inputs := map[string]any{"model_id": agent.ModelID}
	if systemPrompt := strings.TrimSpace(agent.SystemPrompt); systemPrompt != "" {
		inputs["system_prompt"] = systemPrompt
	}
	return inputs
}

func canvasUserKey(userID int64) string {
	return fmt.Sprintf("canvas:%d", userID)
}

type discardEventSink struct{}

func (discardEventSink) Emit(ctx context.Context, event infraai.Event) error {
	return nil
}

func agentSupportsCanvasText(raw string) bool {
	return agentSupportsScene(raw, canvasTextGenerateScene)
}

func agentSupportsChat(raw string) bool {
	return agentSupportsScene(raw, "chat")
}

func agentSupportsScene(raw string, want string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil || len(scenes) == 0 {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == want {
			return true
		}
	}
	return false
}

func userMessageForID(rows []MessageHistory, userMessageID int64) (MessageHistory, bool) {
	for _, row := range rows {
		if row.ID == userMessageID {
			return row, true
		}
	}
	return MessageHistory{}, false
}

func chatRunInputSnapshot(row MessageHistory) (string, *apperror.Error) {
	content := strings.TrimSpace(row.Content)
	meta := ""
	if row.MetaJSON != nil {
		meta = strings.TrimSpace(*row.MetaJSON)
	}
	if content == "" && meta == "" {
		return "", apperror.BadRequestKey("ai.chat.user_message_empty", nil, "用户消息内容为空")
	}
	if meta == "" {
		return row.Content, nil
	}
	if content == "" {
		return meta, nil
	}
	raw, err := json.Marshal(map[string]string{"content": row.Content, "meta_json": meta})
	if err != nil {
		return "", apperror.WrapKey(apperror.CodeInternal, 500, "ai.chat.input_snapshot_failed", nil, "生成AI运行输入快照失败", err)
	}
	return string(raw), nil
}

func chatHistory(rows []MessageHistory, currentUserMessageID int64) []map[string]string {
	return chatHistoryWithLimit(rows, currentUserMessageID, 0)
}

func chatHistoryWithLimit(rows []MessageHistory, currentUserMessageID int64, maxHistory int) []map[string]string {
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	history := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		if row.ID == currentUserMessageID || strings.TrimSpace(row.Content) == "" {
			continue
		}
		role := "assistant"
		if row.Role == enum.AIMessageRoleUser {
			role = "user"
		}
		history = append(history, map[string]string{"role": role, "content": row.Content})
	}
	if maxHistory > 0 && len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	return history
}

func chatInputs(agent AgentEngineConfig, history []MessageHistory, userMessageID int64) map[string]any {
	meta := metaForMessage(history, userMessageID)
	inputs := map[string]any{
		"model_id": agent.ModelID,
		"history":  chatHistoryWithLimit(history, userMessageID, maxHistoryFromMeta(meta)),
	}
	if systemPrompt := strings.TrimSpace(agent.SystemPrompt); systemPrompt != "" {
		inputs["system_prompt"] = systemPrompt
	}
	if len(meta) == 0 {
		return inputs
	}
	if value, ok := meta["runtime_params"].(map[string]any); ok {
		for key, raw := range value {
			if number, ok := numberFromAny(raw); ok {
				inputs[key] = number
			}
		}
	}
	if attachments, ok := meta["attachments"].([]any); ok && len(attachments) > 0 {
		inputs["attachments"] = attachments
	}
	return inputs
}

func maxHistoryFromMeta(meta map[string]any) int {
	value, ok := meta["runtime_params"].(map[string]any)
	if !ok {
		return 0
	}
	number, ok := numberFromAny(value["max_history"])
	if !ok {
		return 0
	}
	n := int(number)
	if n < 1 {
		return 0
	}
	if n > maxHistoryLimit {
		return maxHistoryLimit
	}
	return n
}

func metaForMessage(rows []MessageHistory, userMessageID int64) map[string]any {
	for _, row := range rows {
		if row.ID != userMessageID || row.MetaJSON == nil || strings.TrimSpace(*row.MetaJSON) == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(*row.MetaJSON), &decoded); err != nil {
			return nil
		}
		return decoded
	}
	return nil
}

func numberFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := strconv.ParseFloat(string(v), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func userKey(userID int64) string {
	return fmt.Sprintf("admin:%d", userID)
}

func (s *Service) runtimeTools(ctx context.Context, agentID uint64) ([]RuntimeTool, *apperror.Error) {
	if s == nil || s.toolRuntime == nil {
		return nil, nil
	}
	return s.toolRuntime.ListRuntimeTools(ctx, agentID)
}

func toolDefinitions(tools []RuntimeTool) []infraai.ToolDefinition {
	out := make([]infraai.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, infraai.ToolDefinition{Name: tool.Code, Description: tool.Description, Parameters: tool.ParametersJSON})
	}
	return out
}

func resultToolCalls(result *infraai.ChatResult) []infraai.ToolCall {
	if result == nil || len(result.ToolCalls) == 0 {
		return nil
	}
	return result.ToolCalls
}

func (s *Service) executeToolCalls(ctx context.Context, runID uint64, tools []RuntimeTool, calls []infraai.ToolCall) ([]infraai.ToolOutput, error) {
	if s == nil || s.toolRuntime == nil {
		return nil, apperror.Internal("AI工具运行时未配置")
	}
	toolByCode := make(map[string]RuntimeTool, len(tools))
	for _, tool := range tools {
		toolByCode[tool.Code] = tool
	}
	outputs := make([]infraai.ToolOutput, 0, len(calls))
	for _, call := range calls {
		tool, ok := toolByCode[strings.TrimSpace(call.Name)]
		if !ok {
			return nil, apperror.BadRequest("模型请求了未绑定的AI工具")
		}
		if tool.RiskLevel != "low" {
			return nil, apperror.BadRequest("非低风险AI工具暂不允许自动执行")
		}
		arguments := json.RawMessage(strings.TrimSpace(call.Arguments))
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		result, appErr := s.toolRuntime.Execute(ctx, ToolExecuteInput{RunID: runID, Tool: tool, CallID: call.ID, Arguments: arguments})
		if appErr != nil {
			return nil, appErr
		}
		outputs = append(outputs, infraai.ToolOutput{CallID: result.CallID, Name: result.Name, Output: string(result.Output)})
	}
	return outputs, nil
}
