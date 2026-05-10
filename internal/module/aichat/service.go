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

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
)

const defaultTimeoutLimit = 100
const historyLimit = 20
const maxHistoryLimit = 50

type Dependencies struct {
	Repository    Repository
	Publisher     platformrealtime.Publisher
	EngineFactory EngineFactory
	Secretbox     secretbox.Box
	Now           func() time.Time
}

type Service struct {
	repository    Repository
	publisher     platformrealtime.Publisher
	engineFactory EngineFactory
	secretbox     secretbox.Box
	now           func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repository: deps.Repository, publisher: deps.Publisher, engineFactory: deps.EngineFactory, secretbox: deps.Secretbox, now: now}
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
	startedAt := s.now()
	runID, err := repo.CreateRun(ctx, CreateRunRecord{
		ConversationID:   input.ConversationID,
		RequestID:        input.RequestID,
		UserMessageID:    input.UserMessageID,
		UserID:           input.UserID,
		AgentID:          input.AgentID,
		ProviderID:       int64(agent.ProviderID),
		ModelID:          agent.ModelID,
		ModelDisplayName: agent.ModelDisplayName,
		StartedAt:        startedAt,
	})
	if err != nil {
		_ = s.publishFailed(ctx, input, "创建AI运行记录失败")
		return nil, err
	}
	finishRun := func(status string, msg string, cause error) {
		finishedAt := s.now()
		_ = repo.FinishRun(context.Background(), FinishRunRecord{RunID: runID, Status: status, Message: msg, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
	}
	if err := s.publishStart(ctx, input); err != nil {
		finishRun(statusFromError(ctx, err), err.Error(), err)
		return nil, err
	}
	engine, appErr := s.engineForAgent(ctx, *agent)
	if appErr != nil {
		_ = s.publishFailed(ctx, input, appErr.Message)
		finishRun(enum.AIRunStatusFailed, appErr.Message, appErr)
		return nil, appErr
	}
	history, err := repo.LatestMessages(ctx, input.ConversationID, maxHistoryLimit+1)
	if err != nil {
		msg := "读取AI消息历史失败"
		_ = s.publishFailed(ctx, input, msg)
		finishRun(enum.AIRunStatusFailed, msg, err)
		return nil, err
	}
	userContent, ok := userMessageContent(history, input.UserMessageID)
	if !ok {
		msg := "用户消息不存在"
		_ = s.publishFailed(ctx, input, msg)
		appErr := apperror.BadRequest(msg)
		finishRun(enum.AIRunStatusFailed, msg, appErr)
		return nil, appErr
	}
	sink := &conversationEventSink{service: s, input: input}
	result, err := engine.StreamChat(ctx, platformai.ChatInput{
		AgentID: uint64(input.AgentID),
		UserID:  uint64(input.UserID),
		UserKey: userKey(input.UserID),
		Content: userContent,
		Inputs:  chatInputs(*agent, history, input.UserMessageID),
	}, sink)
	if err != nil {
		msg := err.Error()
		_ = s.publishFailed(ctx, input, msg)
		finishRun(statusFromError(ctx, err), msg, err)
		return nil, err
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
	assistantID, err := repo.InsertAssistantMessage(ctx, AssistantMessageRecord{ConversationID: input.ConversationID, Content: answer, Now: s.now()})
	if err != nil {
		msg := "保存AI助手消息失败"
		_ = s.publishFailed(ctx, input, msg)
		finishRun(enum.AIRunStatusFailed, msg, err)
		return nil, err
	}
	finishedAt := s.now()
	if err := repo.CompleteRun(context.Background(), CompleteRunRecord{RunID: runID, AssistantMessageID: assistantID, PromptTokens: resultTokens(result).Prompt, CompletionTokens: resultTokens(result).Completion, TotalTokens: resultTokens(result).Total, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)}); err != nil {
		msg := "更新AI运行记录失败"
		_ = s.publishFailed(ctx, input, msg)
		return nil, err
	}
	if err := s.publishCompleted(ctx, input, assistantID); err != nil {
		return nil, err
	}
	return &ConversationReplyResult{ConversationID: input.ConversationID, AssistantMessageID: assistantID}, nil
}

type tokenResult struct{ Prompt, Completion, Total int }

func resultTokens(result *platformai.ChatResult) tokenResult {
	if result == nil {
		return tokenResult{}
	}
	return tokenResult{Prompt: result.PromptTokens, Completion: result.CompletionTokens, Total: result.TotalTokens}
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
	count, err := repo.TimeoutRuns(ctx, limit, "AI运行超时")
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

func (s *Service) engineForAgent(ctx context.Context, agent AgentEngineConfig) (platformai.Engine, *apperror.Error) {
	if agent.AgentID == 0 || agent.ProviderID == 0 {
		return nil, apperror.BadRequest("AI智能体或供应商未配置")
	}
	apiKeyEnc := strings.TrimSpace(agent.EngineAPIKeyEnc)
	if apiKeyEnc == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.Internal("AI引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewEngine(ctx, EngineConfig{EngineType: platformai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI引擎失败", err)
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
	return s.publisher.Publish(ctx, platformrealtime.Publication{Platform: enum.PlatformAdmin, UserID: userID, Envelope: event.Envelope})
}

type conversationEventSink struct {
	service *Service
	input   ConversationReplyInput
}

func (s *conversationEventSink) Emit(ctx context.Context, event platformai.Event) error {
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

func agentSupportsChat(raw string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil || len(scenes) == 0 {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == "chat" {
			return true
		}
	}
	return false
}

func userMessageContent(rows []MessageHistory, userMessageID int64) (string, bool) {
	for _, row := range rows {
		if row.ID == userMessageID {
			return row.Content, true
		}
	}
	return "", false
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
		"model_id":      agent.ModelID,
		"system_prompt": agent.SystemPrompt,
		"history":       chatHistoryWithLimit(history, userMessageID, maxHistoryFromMeta(meta)),
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
