package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/taskqueue"

	"github.com/google/uuid"
)

const defaultTimeoutLimit = 100

type Dependencies struct {
	Repository Repository
	Enqueuer   taskqueue.Enqueuer
	Publisher  platformrealtime.Publisher
	Provider   Provider
	Now        func() time.Time
}

type Service struct {
	repository Repository
	enqueuer   taskqueue.Enqueuer
	publisher  platformrealtime.Publisher
	provider   Provider
	now        func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	provider := deps.Provider
	if provider == nil {
		provider = deterministicProvider{}
	}
	return &Service{repository: deps.Repository, enqueuer: deps.Enqueuer, publisher: deps.Publisher, provider: provider, now: now}
}

func (s *Service) CreateRun(ctx context.Context, userID int64, input CreateRunInput) (*CreateRunResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, apperror.BadRequest("消息内容不能为空")
	}
	if input.AgentID <= 0 {
		return nil, apperror.BadRequest("智能体ID不能为空")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	ok, err := repo.ActiveAgentExists(ctx, input.AgentID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "校验AI智能体失败", err)
	}
	if !ok {
		return nil, apperror.BadRequest("关联的智能体不存在或已禁用")
	}
	if input.ConversationID > 0 {
		conversation, err := repo.Conversation(ctx, input.ConversationID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI会话失败", err)
		}
		if conversation == nil {
			return nil, apperror.NotFound("AI会话不存在")
		}
		if conversation.UserID != userID {
			return nil, apperror.Forbidden("无权访问该AI会话")
		}
	}
	meta, appErr := encodeCreateMeta(input)
	if appErr != nil {
		return nil, appErr
	}
	record, err := repo.CreateRun(ctx, CreateRunRecord{
		UserID: userID, AgentID: input.AgentID, ConversationID: input.ConversationID, Content: content,
		RequestID: uuid.NewString(), MetaJSON: meta, Now: s.now(),
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI运行失败", err)
	}
	task, err := NewRunExecuteTask(RunExecutePayload{RunID: record.RunID})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "构建AI运行任务失败", err)
	}
	if s.enqueuer != nil {
		if _, err := s.enqueuer.Enqueue(ctx, task); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "AI运行已创建但入队失败", err)
		}
	}
	res := &CreateRunResponse{ConversationID: record.ConversationID, RunID: record.RunID, RequestID: record.RequestID, UserMessageID: record.UserMessageID, AgentID: record.AgentID, IsNew: record.IsNew}
	event, err := BuildStartEvent(StartPayload{RunID: res.RunID, ConversationID: res.ConversationID, RequestID: res.RequestID, UserMessageID: res.UserMessageID, AgentID: res.AgentID, IsNew: res.IsNew})
	if err == nil {
		s.publish(ctx, userID, event)
	}
	return res, nil
}

func (s *Service) Events(ctx context.Context, userID int64, runID int64, lastID string) (*EventsResponse, *apperror.Error) {
	run, appErr := s.requireOwnedRun(ctx, userID, runID)
	if appErr != nil {
		return nil, appErr
	}
	events := reconstructedEvents(*run, s.assistantForRun(ctx, run))
	filtered := make([]StreamEventItem, 0, len(events))
	last := strings.TrimSpace(lastID)
	latest := last
	for _, event := range events {
		if last != "" && last != "0-0" && !isNewerStreamID(event.ID, last) {
			continue
		}
		filtered = append(filtered, event)
		if latest == "" || isNewerStreamID(event.ID, latest) {
			latest = event.ID
		}
	}
	if latest == "" {
		latest = "0-0"
	}
	errorMsg := ""
	if run.ErrorMsg != nil {
		errorMsg = *run.ErrorMsg
	}
	return &EventsResponse{Events: filtered, LastID: latest, RunStatus: run.RunStatus, Terminal: isTerminal(run.RunStatus), ErrorMsg: errorMsg}, nil
}

func (s *Service) Cancel(ctx context.Context, userID int64, runID int64) (*CancelResponse, *apperror.Error) {
	run, appErr := s.requireOwnedRun(ctx, userID, runID)
	if appErr != nil {
		return nil, appErr
	}
	if run.RunStatus != enum.AIRunStatusRunning {
		return nil, apperror.BadRequest("只能取消运行中的AI任务")
	}
	repo, _ := s.requireRepository()
	if err := repo.MarkCanceled(ctx, runID); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "取消AI运行失败", err)
	}
	event, err := BuildCancelEvent(runID)
	if err == nil {
		s.publish(ctx, userID, event)
	}
	return &CancelResponse{RunID: runID, Status: "canceled"}, nil
}

func (s *Service) ExecuteRun(ctx context.Context, input RunExecuteInput) (*RunExecuteResult, error) {
	if input.RunID <= 0 {
		return nil, apperror.BadRequest("无效的AI运行ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	record, err := repo.RunForExecute(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	if record == nil || record.Run.RunStatus != enum.AIRunStatusRunning {
		return &RunExecuteResult{RunID: input.RunID}, nil
	}
	start := s.now()
	result, err := s.provider.Generate(ctx, GenerateInput{RunID: input.RunID, UserID: record.Run.UserID, AgentID: record.Run.AgentID, Content: record.UserMessageContent})
	if err != nil {
		msg := err.Error()
		_ = repo.MarkFailed(ctx, input.RunID, msg)
		event, buildErr := BuildFailedEvent(input.RunID, msg)
		if buildErr == nil {
			s.publish(ctx, record.Run.UserID, event)
		}
		return nil, err
	}
	if result == nil {
		result = &GenerateResult{}
	}
	latency := int(s.now().Sub(start).Milliseconds())
	message, err := repo.MarkSuccess(ctx, RunSuccessRecord{
		RunID: input.RunID, ConversationID: record.Run.ConversationID, Content: result.Content,
		ModelSnapshot: result.ModelSnapshot, PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, LatencyMS: latency,
	})
	if err != nil {
		return nil, err
	}
	if delta, err := BuildDeltaEvent(input.RunID, result.Content); err == nil {
		s.publish(ctx, record.Run.UserID, delta)
	}
	assistantID := int64(0)
	if message != nil {
		assistantID = message.ID
	}
	userMessageID := int64(0)
	if record.Run.UserMessageID != nil {
		userMessageID = *record.Run.UserMessageID
	}
	if completed, err := BuildCompletedEvent(CompletedPayload{RunID: input.RunID, ConversationID: record.Run.ConversationID, UserMessageID: userMessageID, AssistantMessageID: assistantID}); err == nil {
		s.publish(ctx, record.Run.UserID, completed)
	}
	return &RunExecuteResult{RunID: input.RunID}, nil
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
		return nil, apperror.Internal("AI运行时仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireOwnedRun(ctx context.Context, userID int64, runID int64) (*Run, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if runID <= 0 {
		return nil, apperror.BadRequest("无效的AI运行ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	run, err := repo.RunForUser(ctx, runID, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行失败", err)
	}
	if run == nil {
		return nil, apperror.NotFound("AI运行不存在")
	}
	if run.UserID != userID {
		return nil, apperror.Forbidden("无权访问该AI运行")
	}
	return run, nil
}

func (s *Service) assistantForRun(ctx context.Context, run *Run) *Message {
	if run == nil || run.AssistantMessageID == nil {
		return nil
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil
	}
	message, err := repo.AssistantMessage(ctx, *run.AssistantMessageID)
	if err != nil {
		return nil
	}
	return message
}

func (s *Service) publish(ctx context.Context, userID int64, event EnvelopeEvent) {
	if s == nil || s.publisher == nil || userID <= 0 {
		return
	}
	_ = s.publisher.Publish(ctx, platformrealtime.Publication{Platform: enum.PlatformAdmin, UserID: userID, Envelope: event.Envelope})
}

func encodeCreateMeta(input CreateRunInput) (*string, *apperror.Error) {
	meta := map[string]any{}
	if input.MaxHistory > 0 {
		meta["max_history"] = input.MaxHistory
	}
	if len(input.Attachments) > 0 {
		meta["attachments"] = input.Attachments
	}
	if input.Temperature != nil {
		meta["temperature"] = *input.Temperature
	}
	if input.MaxTokens != nil {
		meta["max_tokens"] = *input.MaxTokens
	}
	if len(meta) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, apperror.BadRequest("AI运行参数格式错误")
	}
	s := string(raw)
	return &s, nil
}

func reconstructedEvents(run Run, assistant *Message) []StreamEventItem {
	events := []StreamEventItem{}
	userMessageID := int64(0)
	if run.UserMessageID != nil {
		userMessageID = *run.UserMessageID
	}
	events = append(events, StreamEventItem{ID: "1-0", Event: EventAIResponseStart, Data: map[string]any{"run_id": run.ID, "conversation_id": run.ConversationID, "request_id": run.RequestID, "user_message_id": userMessageID, "agent_id": run.AgentID}})
	if assistant != nil && strings.TrimSpace(assistant.Content) != "" {
		events = append(events, StreamEventItem{ID: "2-0", Event: EventAIResponseDelta, Data: map[string]any{"run_id": run.ID, "delta": assistant.Content}})
	}
	switch run.RunStatus {
	case enum.AIRunStatusSuccess:
		assistantID := int64(0)
		if run.AssistantMessageID != nil {
			assistantID = *run.AssistantMessageID
		}
		events = append(events, StreamEventItem{ID: "3-0", Event: EventAIResponseCompleted, Data: map[string]any{"run_id": run.ID, "conversation_id": run.ConversationID, "user_message_id": userMessageID, "assistant_message_id": assistantID}})
	case enum.AIRunStatusFail:
		msg := "AI运行失败"
		if run.ErrorMsg != nil && strings.TrimSpace(*run.ErrorMsg) != "" {
			msg = *run.ErrorMsg
		}
		events = append(events, StreamEventItem{ID: "3-0", Event: EventAIResponseFailed, Data: map[string]any{"run_id": run.ID, "msg": msg}})
	case enum.AIRunStatusCanceled:
		events = append(events, StreamEventItem{ID: "3-0", Event: EventAIResponseCancel, Data: map[string]any{"run_id": run.ID}})
	}
	return events
}

func isTerminal(status int) bool {
	return status == enum.AIRunStatusSuccess || status == enum.AIRunStatusFail || status == enum.AIRunStatusCanceled
}

type deterministicProvider struct{}

func (deterministicProvider) Generate(ctx context.Context, input GenerateInput) (*GenerateResult, error) {
	if strings.TrimSpace(input.Content) == "" {
		return nil, errors.New("消息内容不能为空")
	}
	return &GenerateResult{Content: fmt.Sprintf("收到：%s", input.Content), ModelSnapshot: "go-deterministic-provider"}, nil
}
