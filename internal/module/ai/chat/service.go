package aichat

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const defaultTimeoutLimit = 100
const defaultRunStaleTimeout = 15 * time.Minute
const maxHistoryLimit = 50

var (
	ErrAssistantPublisherNotConfigured    = errors.New("assistant publisher is not configured")
	ErrAssistantPublicationRejected       = errors.New("assistant publication rejected by reply command lease")
	ErrProviderAttemptRecorderMissing     = errors.New("provider attempt recorder is not configured")
	ErrProviderAttemptInvalid             = errors.New("provider attempt identity is invalid")
	ErrPaidFinalizationRetry              = errors.New("paid AI finalization must be retried")
	ErrToolRuntimeNotConfigured           = errors.New("AI tool runtime is not configured")
	ErrOfficialModelResolverNotConfigured = errors.New("official model resolver is not configured")
)

type Dependencies struct {
	Repository          Repository
	AssistantPublisher  AssistantPublisher
	DeliveryCommitter   DeliveryCommitter
	AttemptRecorder     ProviderAttemptRecorder
	PaidAttemptExecutor PaidChatAttemptExecutor
	Publisher           infrarealtime.Publisher
	EngineFactory       EngineFactory
	Secretbox           secretbox.Box
	ToolRuntime         ToolRuntime
	KnowledgeRuntime    KnowledgeRuntime
	RunRecorder         RunRecorder
	TextGeneration      TextGeneration
	PricingResolver     officialmodel.Resolver
	RunStaleTimeout     time.Duration
	Now                 func() time.Time
	Logger              *slog.Logger
}

type Service struct {
	repository          Repository
	assistantPublisher  AssistantPublisher
	deliveryCommitter   DeliveryCommitter
	attemptRecorder     ProviderAttemptRecorder
	paidAttemptExecutor PaidChatAttemptExecutor
	publisher           infrarealtime.Publisher
	engineFactory       EngineFactory
	secretbox           secretbox.Box
	toolRuntime         ToolRuntime
	knowledgeRuntime    KnowledgeRuntime
	runRecorder         RunRecorder
	textGeneration      TextGeneration
	pricingResolver     officialmodel.Resolver
	runStaleTimeout     time.Duration
	now                 func() time.Time
	logger              *slog.Logger
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
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repository: deps.Repository, assistantPublisher: deps.AssistantPublisher, deliveryCommitter: deps.DeliveryCommitter, attemptRecorder: deps.AttemptRecorder, paidAttemptExecutor: deps.PaidAttemptExecutor, publisher: deps.Publisher, engineFactory: deps.EngineFactory, secretbox: deps.Secretbox, toolRuntime: deps.ToolRuntime, knowledgeRuntime: deps.KnowledgeRuntime, runRecorder: deps.RunRecorder, textGeneration: deps.TextGeneration, pricingResolver: deps.PricingResolver, runStaleTimeout: runStaleTimeout, now: now, logger: logger}
}

func NewRuntimeService(deps Dependencies) (*Service, error) {
	if deps.ToolRuntime == nil {
		return nil, ErrToolRuntimeNotConfigured
	}
	if deps.PricingResolver == nil {
		return nil, ErrOfficialModelResolverNotConfigured
	}
	if deps.DeliveryCommitter == nil {
		return nil, ErrDeliveryCommitterNotConfigured
	}
	return NewService(deps), nil
}

func (s *Service) ExecuteConversationReply(ctx context.Context, input ConversationReplyInput) (replyResult *ConversationReplyResult, replyErr error) {
	if input.ConversationID <= 0 || input.UserID <= 0 || input.UserMessageID <= 0 || strings.TrimSpace(input.RequestID) == "" {
		return nil, apperror.BadRequest("AI对话回复任务参数错误")
	}
	if input.DeliveryContext == nil {
		input.DeliveryContext = ctx
	}
	input.PrepareStartedAt = s.now()
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
	paidReply := input.CommandID > 0
	var acceptedRun *airun.Run
	if paidReply {
		acceptedRun, err = repo.AcceptedRunForReply(ctx, input.UserID, strings.TrimSpace(input.RequestID))
		if err != nil {
			return nil, err
		}
		if err := validateAcceptedReplyRun(acceptedRun, input); err != nil {
			return nil, err
		}
		if input.AgentID == 0 {
			input.AgentID = acceptedRun.AgentID
		}
	}
	if input.AgentID == 0 {
		input.AgentID = int64(conversation.AgentID)
	}
	if int64(conversation.AgentID) != input.AgentID {
		return nil, apperror.BadRequest("会话智能体不匹配")
	}
	agent, err := repo.AgentForRuntime(ctx, uint64(input.AgentID))
	if err != nil {
		return nil, err
	}
	if agent == nil || agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || agent.ProviderModelStatus != enum.CommonYes || !agentSupportsChat(agent.ScenesJSON) {
		msg := "该智能体不支持对话场景"
		return nil, apperror.BadRequest(msg)
	}
	if _, modelErr := resolveCallableAgentModel(ctx, s.pricingResolver, *agent); modelErr != nil {
		return nil, callableModelError(modelErr)
	}
	if paidReply && (int64(agent.ProviderID) != acceptedRun.ProviderID || strings.TrimSpace(agent.ModelID) != strings.TrimSpace(acceptedRun.ModelID)) {
		return nil, apperror.Internal("AI运行配置与接受快照不一致")
	}
	if err := s.publishStart(input.DeliveryContext, input); err != nil {
		return nil, err
	}
	engine, appErr := s.engineForAgent(ctx, *agent)
	if appErr != nil {
		return nil, appErr
	}
	history, err := repo.LatestMessages(ctx, input.ConversationID, maxHistoryLimit+1)
	if err != nil {
		return nil, err
	}
	userMessage, ok := userMessageForID(history, input.UserMessageID)
	if !ok {
		msg := "用户消息不存在"
		appErr := apperror.BadRequest(msg)
		return nil, appErr
	}
	userContent := userMessage.Content
	inputSnapshot, appErr := chatRunInputSnapshot(userMessage)
	if appErr != nil {
		return nil, appErr
	}
	if !paidReply && s.runRecorder == nil {
		msg := "AI运行记录服务未配置"
		return nil, apperror.Internal(msg)
	}
	startedAt := s.now()
	runID := int64(0)
	if paidReply {
		runID = acceptedRun.ID
		if acceptedRun.StartedAt != nil && !acceptedRun.StartedAt.IsZero() {
			startedAt = *acceptedRun.StartedAt
		}
	} else {
		conversationID := input.ConversationID
		userMessageID := input.UserMessageID
		runID, err = s.runRecorder.Start(ctx, airun.StartInput{
			Platform: enum.PlatformAdmin, ConversationID: &conversationID, UserMessageID: &userMessageID,
			RequestID: input.RequestID, UserID: input.UserID, AgentID: input.AgentID, ProviderID: int64(agent.ProviderID),
			ModelID: agent.ModelID, ModelDisplayName: agent.ModelDisplayName, InputSnapshot: inputSnapshot, StartedAt: startedAt,
		})
		if err != nil {
			return nil, err
		}
	}
	finishRun := func(status string, msg string, cause error) {
		if paidReply {
			return
		}
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
	delivery := newDeliverySink(deliverySinkOptions{
		DeliveryContext: input.DeliveryContext,
		Committer:       s.deliveryCommitter,
		Publisher:       s.publisher,
		CommandID:       input.CommandID,
		Owner:           input.LeaseOwner,
		Token:           input.LeaseToken,
		ConversationID:  input.ConversationID,
		UserID:          input.UserID,
		RequestID:       input.RequestID,
		Now:             s.now,
	})
	defer func() {
		if closeErr := delivery.Close(context.WithoutCancel(ctx)); closeErr != nil {
			if replyErr != nil {
				replyErr = errors.Join(replyErr, closeErr)
				return
			}
			replyResult = nil
			replyErr = closeErr
		}
	}()
	sink := infraai.EventSink(delivery)
	chatInput := infraai.ChatInput{
		AgentID: uint64(input.AgentID),
		RunID:   uint64(runID),
		UserID:  uint64(input.UserID),
		UserKey: userKey(input.UserID),
		Content: userContent,
		Inputs:  chatInputs(*agent, history, input.UserMessageID),
	}
	if paidReply && s.paidAttemptExecutor != nil {
		identity, identityErr := paidReplyRequestIdentity(acceptedRun, input, userMessage)
		if identityErr != nil {
			result, finalizationErr := s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, true)
			if finalizationErr != nil {
				return nil, errors.Join(identityErr, finalizationErr)
			}
			return result, nil
		}
		input.RequestIdentity = identity
	}
	runtimeTools, appErr := s.runtimeTools(ctx, uint64(input.AgentID))
	if appErr != nil {
		if paidReply {
			return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, true)
		}
		finishRun(enum.AIRunStatusFailed, appErr.Message, appErr)
		return nil, appErr
	}
	chatInput.Tools = toolDefinitions(runtimeTools)
	paidResult, err := s.streamChatWithAttempt(ctx, runID, input, engine, chatInput, sink)
	if flushErr := delivery.Flush(context.WithoutCancel(ctx)); flushErr != nil {
		if err != nil {
			err = errors.Join(err, flushErr)
		} else {
			err = flushErr
		}
	}
	if paidResult != nil && paidResult.Finalized {
		return finalizedConversationReply(input, paidResult), nil
	}
	if err != nil {
		msg := err.Error()
		finishRun(statusFromError(ctx, err), msg, err)
		return nil, err
	}
	result := paidResult.ChatResult
	if input.CommandID > 0 && deliveryStopped(input.DeliveryContext) {
		return &ConversationReplyResult{ConversationID: input.ConversationID, DeliveryStopped: true}, nil
	}
	if toolCalls := resultToolCalls(result); len(toolCalls) > 0 {
		if appErr := validateRunUsageStatus(result); appErr != nil {
			if paidReply {
				return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
			}
			msg := appErr.Message
			finishRun(enum.AIRunStatusFailed, msg, appErr)
			return nil, appErr
		}
		firstUsage := resultTokens(result)
		input.PrepareStartedAt = s.now()
		outputs, toolErr := s.executeToolCalls(ctx, uint64(runID), runtimeTools, toolCalls)
		if toolErr != nil {
			if paidReply {
				return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
			}
			msg := toolErr.Error()
			finishRun(enum.AIRunStatusFailed, msg, toolErr)
			return nil, toolErr
		}
		if input.CommandID > 0 && deliveryStopped(input.DeliveryContext) {
			return &ConversationReplyResult{ConversationID: input.ConversationID, DeliveryStopped: true}, nil
		}
		chatInput.ToolCalls = toolCalls
		chatInput.ToolOutputs = outputs
		paidResult, err = s.streamChatWithAttempt(ctx, runID, input, engine, chatInput, sink)
		if flushErr := delivery.Flush(context.WithoutCancel(ctx)); flushErr != nil {
			if err != nil {
				err = errors.Join(err, flushErr)
			} else {
				err = flushErr
			}
		}
		if paidResult != nil && paidResult.Finalized {
			return finalizedConversationReply(input, paidResult), nil
		}
		if err != nil {
			msg := err.Error()
			finishRun(statusFromError(ctx, err), msg, err)
			return nil, err
		}
		result = paidResult.ChatResult
		if input.CommandID > 0 && deliveryStopped(input.DeliveryContext) {
			return &ConversationReplyResult{ConversationID: input.ConversationID, DeliveryStopped: true}, nil
		}
		if appErr := validateRunUsageStatus(result); appErr != nil {
			if paidReply {
				return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
			}
			msg := appErr.Message
			finishRun(enum.AIRunStatusFailed, msg, appErr)
			return nil, appErr
		}
		addTokenUsage(result, firstUsage)
		if len(resultToolCalls(result)) > 0 {
			msg := "工具调用轮次超过MVP限制"
			appErr := apperror.BadRequest(msg)
			if paidReply {
				return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
			}
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
		if err := delivery.Accept(answer); err != nil {
			return nil, err
		}
	}
	if err := delivery.Flush(context.WithoutCancel(ctx)); err != nil {
		return nil, err
	}
	if appErr := validateRunUsageStatus(result); appErr != nil {
		if paidReply {
			return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
		}
		msg := appErr.Message
		finishRun(enum.AIRunStatusFailed, msg, appErr)
		return nil, appErr
	}
	if input.CommandID > 0 && deliveryStopped(input.DeliveryContext) {
		return &ConversationReplyResult{ConversationID: input.ConversationID, DeliveryStopped: true}, nil
	}
	if s.assistantPublisher == nil {
		msg := "AI助手消息发布器未配置"
		if paidReply {
			return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
		}
		finishRun(enum.AIRunStatusFailed, msg, ErrAssistantPublisherNotConfigured)
		return nil, ErrAssistantPublisherNotConfigured
	}
	assistantID, published, err := s.assistantPublisher.PublishAssistant(ctx, AssistantPublication{CommandID: input.CommandID, ConversationID: input.ConversationID, Owner: input.LeaseOwner, Token: input.LeaseToken, Content: answer, Now: s.now()})
	if err != nil {
		msg := "保存AI助手消息失败"
		if paidReply {
			return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
		}
		finishRun(enum.AIRunStatusFailed, msg, err)
		return nil, err
	}
	if !published || assistantID <= 0 {
		msg := "AI助手消息发布租约已失效"
		if paidReply {
			return s.finalizePaidFailure(context.WithoutCancel(ctx), runID, input, false)
		}
		finishRun(enum.AIRunStatusCanceled, msg, ErrAssistantPublicationRejected)
		return nil, ErrAssistantPublicationRejected
	}
	finishedAt := s.now()
	assistantMessageID := assistantID
	tokens := resultTokens(result)
	if !paidReply {
		if err := s.runRecorder.Complete(context.Background(), airun.CompleteInput{RunID: runID, AssistantMessageID: &assistantMessageID, PromptTokens: tokens.Prompt, CompletionTokens: tokens.Completion, TotalTokens: tokens.Total, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)}); err != nil {
			s.logger.WarnContext(context.WithoutCancel(ctx), "AI run completion recording failed after durable reply commit",
				"command_id", input.CommandID, "run_id", runID, "assistant_message_id", assistantID, "error", err)
		}
	}
	return &ConversationReplyResult{ConversationID: input.ConversationID, AssistantMessageID: assistantID}, nil
}

func validateAcceptedReplyRun(run *airun.Run, input ConversationReplyInput) error {
	if run == nil || run.ID <= 0 || run.ConversationID == nil || *run.ConversationID != input.ConversationID || run.UserMessageID == nil || *run.UserMessageID != input.UserMessageID || run.UserID != input.UserID || strings.TrimSpace(run.RequestID) != strings.TrimSpace(input.RequestID) || run.AgentID <= 0 || run.ProviderID <= 0 || strings.TrimSpace(run.ModelID) == "" || run.Status != enum.AIRunStatusRunning {
		return apperror.Internal("AI回复任务缺少可执行的计费运行")
	}
	if run.BillingStatus != "pending" && run.BillingStatus != "held" {
		return apperror.Internal("AI回复任务计费状态不可执行")
	}
	return nil
}

func replyRunIdempotencyKey(commandID uint64) string {
	if commandID == 0 {
		return ""
	}
	return "reply-command:" + strconv.FormatUint(commandID, 10)
}

func (s *Service) CompleteText(ctx context.Context, input TextCompletionInput) (*TextCompletionResponse, *apperror.Error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.Message = strings.TrimSpace(input.Message)
	input.ModelID = strings.TrimSpace(input.ModelID)
	if !enum.IsRegisteredPlatform(input.Platform) {
		return nil, apperror.BadRequestKey("aitext.platform.invalid", nil, "无效的文本生成平台")
	}
	if input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 {
		return nil, apperror.New(aitext.ErrorCodeRequestInvalid, apperror.CategoryValidation, 400, apperror.Permanent, "", nil, "request_id不能为空")
	}
	if input.UserID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.AgentID <= 0 || input.Message == "" {
		return nil, apperror.BadRequestKey("aitext.request.invalid", nil, "文本生成参数错误")
	}
	if s == nil || s.textGeneration == nil {
		return nil, apperror.New(aitext.ErrorCodeConfiguration, apperror.CategoryInternal, 500, apperror.Permanent, "", nil, "AI文本任务服务未配置")
	}
	replay, found, replayErr := s.textGeneration.ReplayAndWait(ctx, aitext.ReplayInput{
		UserID: int64(input.UserID), RequestID: input.RequestID, AgentID: uint64(input.AgentID),
		Operation: "text.generate", Modality: "text", NormalizedText: input.Message,
	})
	if replayErr != nil {
		return nil, replayErr
	}
	if found {
		return textCompletionResult(replay)
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, apperror.InternalKey("aitext.repository_missing", nil, "AI文本生成仓储未配置")
	}
	agent, err := repo.AgentForRuntime(ctx, uint64(input.AgentID))
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aitext.agent_query_failed", nil, "查询文本智能体失败", err)
	}
	if agent == nil || agent.AgentID == 0 {
		return nil, apperror.NotFoundKey("aitext.agent_not_found", nil, "文本智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || agent.ProviderModelStatus != enum.CommonYes || !agentSupportsTextGeneration(agent.ScenesJSON) {
		return nil, apperror.BadRequestKey("aitext.agent_unavailable", nil, "该智能体不支持文本生成")
	}
	pricingSnapshotJSON, effectiveMaxOutputTokens, appErr := s.textCompletionPricingSnapshot(ctx, *agent)
	if appErr != nil {
		return nil, appErr
	}
	normalizedText := input.Message
	fingerprint, err := requestidentity.BuildFingerprint(requestidentity.Input{
		UserID: int64(input.UserID), Operation: "text.generate", Modality: "text", AgentID: int64(agent.AgentID),
		ModelID: strings.TrimSpace(agent.ModelID), NormalizedText: normalizedText,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: effectiveMaxOutputTokens},
	})
	if err != nil {
		return nil, apperror.Wrap(aitext.ErrorCodeRequestInvalid, apperror.CategoryValidation, 400, apperror.Permanent, "", nil, "AI文本请求身份无效", err)
	}
	inputSnapshot, err := aitext.EncodeProviderInputSnapshot(aitext.ProviderInputSnapshot{
		Operation: "text.generate", Modality: "text", NormalizedText: normalizedText,
		MaxOutputTokens: effectiveMaxOutputTokens, Prompt: input.Message, SystemPrompt: strings.TrimSpace(agent.SystemPrompt),
	})
	if err != nil {
		return nil, apperror.Wrap(aitext.ErrorCodeConfiguration, apperror.CategoryInternal, 500, apperror.Permanent, "", nil, "生成AI文本输入快照失败", err)
	}
	result, appErr := s.textGeneration.SubmitAndWait(ctx, aitext.AcceptInput{
		Platform: enum.PlatformAdmin, UserID: input.UserID, RequestID: input.RequestID, RequestFingerprint: fingerprint,
		Kind: aitext.KindText, AgentID: agent.AgentID, ProviderID: agent.ProviderID,
		ModelID: strings.TrimSpace(agent.ModelID), ModelDisplayName: strings.TrimSpace(agent.ModelDisplayName),
		Prompt: input.Message, InputSnapshot: inputSnapshot, PricingSnapshotJSON: pricingSnapshotJSON,
		EffectiveMaxOutputTokens: effectiveMaxOutputTokens,
	})
	if appErr != nil {
		return nil, appErr
	}
	return textCompletionResult(result)
}

func textCompletionResult(result *aitext.Result) (*TextCompletionResponse, *apperror.Error) {
	if result == nil || result.TaskID == 0 || result.Kind != aitext.KindText || strings.TrimSpace(result.Answer) == "" {
		return nil, apperror.BadRequestKey("aitext.empty_result", nil, "AI文本生成结果为空")
	}
	return &TextCompletionResponse{ID: fmt.Sprintf("text-completion-%d", result.TaskID), Object: "chat.completion", Content: strings.TrimSpace(result.Answer)}, nil
}

func (s *Service) textCompletionPricingSnapshot(ctx context.Context, agent AgentEngineConfig) (string, int64, *apperror.Error) {
	if s == nil || s.pricingResolver == nil {
		return "", 0, apperror.Wrap(aitext.ErrorCodePriceUnavailable, apperror.CategoryInternal, 500, apperror.Permanent, "", nil, "AI模型价格服务未配置", officialmodel.ErrRepositoryNotConfigured)
	}
	model, err := resolveCallableAgentModel(ctx, s.pricingResolver, agent)
	if err != nil {
		return "", 0, apperror.Wrap(aitext.ErrorCodePriceUnavailable, apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该智能体缺少可用的模型价格", err)
	}
	if agent.BillingMultiplierPPM <= 0 || strings.TrimSpace(agent.EngineType) == "" {
		return "", 0, apperror.New(aitext.ErrorCodeConfiguration, apperror.CategoryValidation, 400, apperror.Permanent, "", nil, "AI文本计费配置无效")
	}
	effective := model.Model.MaxOutputTokens
	if effective <= 0 || effective > int64(^uint(0)>>1) {
		return "", 0, apperror.Wrap(aitext.ErrorCodeUnsafeUpperBound, apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "AI文本输出上限不安全", officialmodel.ErrInvalidCatalog)
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: strings.TrimSpace(agent.EngineType), RequestedModelID: strings.TrimSpace(agent.ModelID),
		EffectiveMaxOutputTokens: int(effective), MultiplierPPM: agent.BillingMultiplierPPM,
	})
	if err != nil {
		return "", 0, apperror.Wrap(aitext.ErrorCodePriceUnavailable, apperror.CategoryInternal, 500, apperror.Permanent, "", nil, "生成AI模型价格快照失败", err)
	}
	return raw, effective, nil
}

func resolveCallableAgentModel(ctx context.Context, resolver officialmodel.Resolver, agent AgentEngineConfig) (officialmodel.ResolvedModel, error) {
	return officialmodel.ResolveMappedRoute(ctx, resolver, agent.ModelID, agent.OfficialModelID, agent.OfficialCatalogVersion, agent.MappingStatus)
}

func callableModelError(err error) *apperror.Error {
	if errors.Is(err, officialmodel.ErrModelRetired) {
		return apperror.Wrap("ai.official_model.retired", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该官方模型已退役", err)
	}
	return apperror.Wrap("ai.official_model.unavailable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该供应商模型未映射到当前官方模型目录", err)
}

func finalizedConversationReply(input ConversationReplyInput, result *PaidChatAttemptResult) *ConversationReplyResult {
	return &ConversationReplyResult{
		ConversationID: input.ConversationID, AssistantMessageID: result.AssistantMessageID,
		DeliveryStopped: deliveryStopped(input.DeliveryContext), Finalized: true,
	}
}

func (s *Service) finalizePaidFailure(ctx context.Context, runID int64, input ConversationReplyInput, beforeDispatch bool) (*ConversationReplyResult, error) {
	if s == nil || s.paidAttemptExecutor == nil || runID <= 0 || input.CommandID == 0 {
		return nil, ErrProviderAttemptRecorderMissing
	}
	finalizer, ok := s.paidAttemptExecutor.(PaidChatAttemptFailureFinalizer)
	if !ok {
		return nil, ErrProviderAttemptRecorderMissing
	}
	finalizationInput := PaidChatAttemptInput{
		RunID: runID, CommandID: input.CommandID, LeaseOwner: input.LeaseOwner, LeaseToken: input.LeaseToken,
		RequestID: input.RequestID, DeliveryContext: input.DeliveryContext,
		CommandAttempt: input.CommandAttempt, CommandMaxAttempts: input.CommandMaxAttempts,
	}
	var (
		result *PaidChatAttemptResult
		err    error
	)
	if beforeDispatch {
		result, err = finalizer.FinalizePaidChatPreDispatchFailure(ctx, finalizationInput)
	} else {
		result, err = finalizer.FinalizePaidChatLocalFailure(ctx, finalizationInput)
	}
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Finalized {
		return nil, ErrProviderAttemptInvalid
	}
	return finalizedConversationReply(input, result), nil
}

func (s *Service) streamChatWithAttempt(ctx context.Context, runID int64, input ConversationReplyInput, engine infraai.Engine, chatInput infraai.ChatInput, sink infraai.EventSink) (*PaidChatAttemptResult, error) {
	if input.CommandID == 0 {
		result, err := engine.StreamChat(ctx, chatInput, sink)
		return &PaidChatAttemptResult{ChatResult: result}, err
	}
	if s.paidAttemptExecutor != nil {
		return s.paidAttemptExecutor.ExecutePaidChatAttempt(ctx, PaidChatAttemptInput{
			RunID:              runID,
			CommandID:          input.CommandID,
			LeaseOwner:         input.LeaseOwner,
			LeaseToken:         input.LeaseToken,
			RequestID:          input.RequestID,
			RequestIdentity:    input.RequestIdentity,
			DeliveryContext:    input.DeliveryContext,
			PrepareStartedAt:   input.PrepareStartedAt,
			CommandAttempt:     input.CommandAttempt,
			CommandMaxAttempts: input.CommandMaxAttempts,
			Engine:             engine,
			ChatInput:          chatInput,
			Sink:               sink,
		})
	}
	if s.attemptRecorder == nil {
		return nil, ErrProviderAttemptRecorderMissing
	}
	attempt, err := s.attemptRecorder.PrepareProviderAttempt(ctx, ProviderAttemptPrepareInput{
		RunID:     runID,
		CommandID: input.CommandID,
		Owner:     input.LeaseOwner,
		Token:     input.LeaseToken,
		Now:       s.now(),
	})
	if err != nil {
		return nil, err
	}
	if attempt == nil || attempt.ID == 0 || strings.TrimSpace(attempt.IdempotencyKey) == "" {
		return nil, ErrProviderAttemptInvalid
	}
	if err := s.attemptRecorder.MarkProviderAttemptDispatched(ctx, ProviderAttemptMarkInput{
		RunID:     runID,
		AttemptID: attempt.ID,
		CommandID: input.CommandID,
		Owner:     input.LeaseOwner,
		Token:     input.LeaseToken,
		Now:       s.now(),
	}); err != nil {
		return nil, err
	}
	chatInput.AttemptID = attempt.ID
	chatInput.IdempotencyKey = attempt.IdempotencyKey
	result, providerErr := engine.StreamChat(ctx, chatInput, sink)
	finish := ProviderAttemptFinishInput{
		AttemptID: attempt.ID,
		RunID:     runID,
		CommandID: input.CommandID,
		Owner:     input.LeaseOwner,
		Token:     input.LeaseToken,
		Now:       s.now(),
	}
	if providerErr == nil {
		finish.State = ProviderAttemptSucceeded
		finish.ResponseSHA256 = providerAttemptResponseHash(result)
		if result != nil {
			finish.ProviderRequestID = result.ProviderRequestID
			finish.DispatchState = result.DispatchState
			if finish.DispatchState == "" {
				finish.DispatchState = infraai.DispatchStateDispatched
			}
			usage := result.Usage
			if result.Usage.Complete() {
				finish.UsageStatus = infraai.UsageStatusComplete
			} else {
				finish.UsageStatus = infraai.UsageStatusUnavailable
				usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
			}
			usageJSON, marshalErr := json.Marshal(usage)
			if marshalErr != nil {
				return nil, marshalErr
			}
			finish.UsageJSON = string(usageJSON)
			candidateJSON, marshalErr := marshalChatResultCandidate(result)
			if marshalErr != nil {
				return nil, marshalErr
			}
			finish.ResultCandidateJSON = candidateJSON
		}
		if deliveryStopped(input.DeliveryContext) {
			finish.State = ProviderAttemptCanceled
			finish.ErrorCode = "ai.user_stopped"
		}
	} else {
		finish.State = ProviderAttemptOutcomeUnknown
		finish.ErrorCode = "ai.provider_outcome_unknown"
		finish.DispatchState = infraai.DispatchStateUnknown
		finish.UsageStatus = infraai.UsageStatusUnavailable
		finish.UsageJSON = `{"status":"unavailable"}`
		finish.ProviderRequestID = infraai.ProviderRequestIDFromError(providerErr)
		if outcome, ok := infraai.ProviderOutcomeFromError(providerErr); ok {
			switch outcome {
			case infraai.ProviderOutcomeNotDispatched:
				finish.State = ProviderAttemptFailed
				finish.ErrorCode = "ai.provider_failed"
				finish.DispatchState = infraai.DispatchStateNotDispatched
				if errors.Is(context.Cause(ctx), infraai.ErrCanceled) {
					finish.State = ProviderAttemptCanceled
					finish.ErrorCode = "ai.provider_canceled"
				}
			case infraai.ProviderOutcomeRejected:
				finish.State = ProviderAttemptFailed
				finish.ErrorCode = "ai.provider_failed"
				finish.DispatchState = infraai.DispatchStateDispatched
			}
		}
	}
	if finishErr := s.attemptRecorder.FinishProviderAttempt(context.WithoutCancel(ctx), finish); finishErr != nil {
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, finish.ProviderRequestID, errors.Join(providerErr, finishErr))
	}
	return &PaidChatAttemptResult{ChatResult: result}, providerErr
}

// FinalizeConversationReply is the runner's pre-dispatch cancellation hook.
// The durable paid executor derives the persisted trigger under settlement
// locks, so this does not bypass billing or command finalization.
func (s *Service) FinalizeConversationReply(ctx context.Context, input ConversationReplyInput) (*ConversationReplyResult, error) {
	if input.CommandID == 0 || s == nil || s.paidAttemptExecutor == nil {
		return nil, ErrProviderAttemptRecorderMissing
	}
	finalizer, ok := s.paidAttemptExecutor.(PaidChatAttemptFinalizer)
	if !ok {
		return nil, ErrProviderAttemptRecorderMissing
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	run, err := repo.AcceptedRunForReply(ctx, input.UserID, strings.TrimSpace(input.RequestID))
	if err != nil {
		return nil, err
	}
	if err := validateAcceptedReplyRun(run, input); err != nil {
		return nil, err
	}
	result, err := finalizer.FinalizePaidChatAttempt(ctx, PaidChatAttemptInput{
		RunID: run.ID, CommandID: input.CommandID, LeaseOwner: input.LeaseOwner, LeaseToken: input.LeaseToken,
		RequestID: input.RequestID, DeliveryContext: input.DeliveryContext,
		CommandAttempt: input.CommandAttempt, CommandMaxAttempts: input.CommandMaxAttempts,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Finalized {
		return nil, ErrProviderAttemptInvalid
	}
	return finalizedConversationReply(input, result), nil
}

type chatResultCandidate struct {
	Version   string             `json:"version"`
	Answer    string             `json:"answer,omitempty"`
	ToolCalls []infraai.ToolCall `json:"tool_calls,omitempty"`
}

func marshalChatResultCandidate(result *infraai.ChatResult) (*string, error) {
	if result == nil {
		return nil, nil
	}
	answer := strings.TrimSpace(result.Answer)
	if answer == "" && len(result.ToolCalls) == 0 {
		answer = "AI没有返回内容"
	}
	raw, err := json.Marshal(chatResultCandidate{
		Version:   chatResultCandidateVersion,
		Answer:    answer,
		ToolCalls: result.ToolCalls,
	})
	if err != nil {
		return nil, err
	}
	value := string(raw)
	return &value, nil
}

// MarshalChatResultCandidate serializes the immutable business-result
// candidate stored beside a paid provider attempt. Runtime Gateway adapters
// use this encoder without duplicating the chat result schema.
func MarshalChatResultCandidate(result *infraai.ChatResult) (*string, error) {
	return marshalChatResultCandidate(result)
}

func providerAttemptResponseHash(result *infraai.ChatResult) string {
	if result == nil || result.ResponseSHA256 == ([32]byte{}) {
		return ""
	}
	return hex.EncodeToString(result.ResponseSHA256[:])
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

func (s *Service) publishStart(ctx context.Context, input ConversationReplyInput) error {
	event, err := BuildStartEvent(StartPayload{ConversationID: input.ConversationID, RequestID: input.RequestID, UserMessageID: input.UserMessageID, AgentID: input.AgentID})
	if err != nil {
		return err
	}
	return s.publish(ctx, input.UserID, event)
}

func (s *Service) publish(ctx context.Context, userID int64, event infrarealtime.Envelope) error {
	if s.publisher == nil {
		return nil
	}
	return s.publisher.Publish(ctx, infrarealtime.Publication{Platform: enum.PlatformAdmin, UserID: userID, Envelope: event})
}

func agentSupportsTextGeneration(raw string) bool {
	return agentSupportsScene(raw, capability.SceneTextGenerate)
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

func paidReplyRequestIdentity(run *airun.Run, input ConversationReplyInput, message MessageHistory) (requestidentity.Input, error) {
	if run == nil || len(run.RequestFingerprint) != 32 {
		return requestidentity.Input{}, apperror.Internal("AI运行缺少可重放请求身份")
	}
	snapshot, err := aigateway.ParsePricingSnapshot(run.PricingSnapshotJSON)
	if err != nil {
		return requestidentity.Input{}, apperror.Internal("AI运行价格快照无效")
	}
	meta := metaForMessage([]MessageHistory{message}, message.ID)
	options := requestidentity.GenerationOptions{
		MaxOutputTokens: int64(snapshot.EffectiveMaxOutputTokens),
		Extra:           map[string]string{},
	}
	if params, ok := meta["runtime_params"].(map[string]any); ok {
		for key, raw := range params {
			value, ok := numberFromAny(raw)
			if !ok {
				return requestidentity.Input{}, apperror.Internal("AI运行参数快照无效")
			}
			if key == "max_tokens" {
				if int64(value) != options.MaxOutputTokens {
					return requestidentity.Input{}, apperror.Internal("AI运行输出上限与价格快照不一致")
				}
				continue
			}
			options.Extra[key] = strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	if len(options.Extra) == 0 {
		options.Extra = nil
	}
	attachments := make([]requestidentity.AttachmentIdentity, 0)
	if values, ok := meta["attachments"].([]any); ok {
		attachments = make([]requestidentity.AttachmentIdentity, 0, len(values))
		for _, raw := range values {
			item, ok := raw.(map[string]any)
			if !ok {
				return requestidentity.Input{}, apperror.Internal("AI附件身份快照无效")
			}
			if objectKey, ok := item["object_key"].(string); ok && strings.TrimSpace(objectKey) != "" {
				attachments = append(attachments, requestidentity.AttachmentIdentity{StorageProvider: "cos", StorageKey: strings.TrimSpace(objectKey)})
				continue
			}
			if url, ok := item["url"].(string); ok && strings.TrimSpace(url) != "" {
				attachments = append(attachments, requestidentity.AttachmentIdentity{StorageProvider: "url", StorageKey: strings.TrimSpace(url)})
				continue
			}
			return requestidentity.Input{}, apperror.Internal("AI附件身份快照无效")
		}
	}
	identity := requestidentity.Input{
		UserID:         input.UserID,
		Operation:      "chat.reply",
		Modality:       "chat",
		AgentID:        run.AgentID,
		ModelID:        run.ModelID,
		NormalizedText: message.Content,
		Attachments:    attachments,
		Options:        options,
		ConversationID: input.ConversationID,
	}
	var persisted [32]byte
	copy(persisted[:], run.RequestFingerprint)
	if paidReplyIdentityMatches(run.RequestIdentityStatus, persisted, identity) {
		return identity, nil
	}
	context, err := historyRequestIdentityFromSnapshot(run.InputSnapshot)
	if err != nil {
		return requestidentity.Input{}, apperror.Internal("AI历史请求身份快照无效")
	}
	if context != nil {
		identity.Operation = context.Operation
		identity.SourceMessageID = context.SourceMessageID
		if paidReplyIdentityMatches(run.RequestIdentityStatus, persisted, identity) {
			return identity, nil
		}
	}
	return requestidentity.Input{}, apperror.Internal("AI请求身份与接受快照不一致")
}

func paidReplyIdentityMatches(status string, persisted [32]byte, identity requestidentity.Input) bool {
	fingerprint, err := requestidentity.Fingerprint(identity)
	if err != nil {
		return false
	}
	return requestidentity.CompareForReplay(requestidentity.IdentityStatus(status), persisted, fingerprint) == nil
}

type historyRequestIdentitySnapshot struct {
	Operation       string `json:"operation"`
	SourceMessageID int64  `json:"source_message_id"`
}

func historyRequestIdentityFromSnapshot(raw string) (*historyRequestIdentitySnapshot, error) {
	var envelope struct {
		RequestIdentity json.RawMessage `json:"request_identity"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &envelope); err != nil || len(envelope.RequestIdentity) == 0 || string(envelope.RequestIdentity) == "null" {
		return nil, nil
	}
	var snapshot historyRequestIdentitySnapshot
	if err := json.Unmarshal(envelope.RequestIdentity, &snapshot); err != nil {
		return nil, err
	}
	snapshot.Operation = strings.TrimSpace(snapshot.Operation)
	if (snapshot.Operation != "chat.revision" && snapshot.Operation != "chat.regeneration") || snapshot.SourceMessageID <= 0 {
		return nil, errors.New("invalid history request identity context")
	}
	return &snapshot, nil
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
