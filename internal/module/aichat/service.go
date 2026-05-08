package aichat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
	"admin_back_go/internal/platform/taskqueue"

	"github.com/google/uuid"
)

const defaultTimeoutLimit = 100
const eventsPollStep = 100 * time.Millisecond

type Dependencies struct {
	Repository    Repository
	Enqueuer      taskqueue.Enqueuer
	Publisher     platformrealtime.Publisher
	EngineFactory EngineFactory
	Secretbox     secretbox.Box
	Now           func() time.Time
}

type Service struct {
	repository    Repository
	enqueuer      taskqueue.Enqueuer
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
	return &Service{
		repository:    deps.Repository,
		enqueuer:      deps.Enqueuer,
		publisher:     deps.Publisher,
		engineFactory: deps.EngineFactory,
		secretbox:     deps.Secretbox,
		now:           now,
	}
}

func (s *Service) CreateRun(ctx context.Context, userID int64, input CreateRunInput) (*CreateRunResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, apperror.BadRequest("消息内容不能为空")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	app, appErr := s.resolveAppForCreate(ctx, repo, userID, &input)
	if appErr != nil {
		return nil, appErr
	}
	meta, appErr := encodeCreateMeta(input)
	if appErr != nil {
		return nil, appErr
	}
	record, err := repo.CreateRun(ctx, CreateRunRecord{
		UserID: userID, AgentID: int64(app.AppID), ConversationID: input.ConversationID, Content: content,
		RequestID: uuid.NewString(), MetaJSON: meta, Now: s.now(),
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI运行失败", err)
	}
	sink := newPersistentSink(repo, s.publisher, userID, record.RunID, s.now)
	if event, err := BuildStartEvent(StartPayload{RunID: record.RunID, ConversationID: record.ConversationID, RequestID: record.RequestID, UserMessageID: record.UserMessageID, AgentID: int64(app.AppID), IsNew: record.IsNew}); err == nil {
		_ = sink.emitEnvelope(ctx, event, "")
	}
	task, err := NewRunExecuteTask(RunExecutePayload{RunID: record.RunID})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "构建AI运行任务失败", err)
	}
	if s.enqueuer != nil {
		if _, err := s.enqueuer.Enqueue(ctx, task); err != nil {
			msg := "AI运行已创建但入队失败"
			_ = repo.MarkFailed(ctx, record.RunID, msg)
			if event, buildErr := BuildFailedEvent(record.RunID, msg); buildErr == nil {
				_ = sink.emitEnvelope(ctx, event, msg)
			}
			return nil, apperror.Wrap(apperror.CodeInternal, 500, msg, err)
		}
	}
	res := &CreateRunResponse{ConversationID: record.ConversationID, RunID: record.RunID, RequestID: record.RequestID, UserMessageID: record.UserMessageID, AgentID: int64(app.AppID), AppID: app.AppID, IsNew: record.IsNew}
	return res, nil
}

func (s *Service) Events(ctx context.Context, userID int64, runID int64, lastID string, timeout time.Duration) (*EventsResponse, *apperror.Error) {
	if timeout < 0 {
		timeout = 0
	}

	deadline := time.Now().Add(timeout)
	for {
		res, terminal, appErr := s.eventsOnce(ctx, userID, runID, lastID)
		if appErr != nil || terminal || len(res.Events) > 0 || timeout <= 0 || !time.Now().Before(deadline) {
			return res, appErr
		}

		wait := eventsPollStep
		if remaining := time.Until(deadline); remaining < wait {
			wait = remaining
		}
		if wait <= 0 {
			return res, nil
		}

		select {
		case <-ctx.Done():
			return res, nil
		case <-time.After(wait):
		}
	}
}

func (s *Service) eventsOnce(ctx context.Context, userID int64, runID int64, lastID string) (*EventsResponse, bool, *apperror.Error) {
	run, appErr := s.requireOwnedRun(ctx, userID, runID)
	if appErr != nil {
		return nil, false, appErr
	}
	repo, _ := s.requireRepository()
	rows, err := repo.ListRunEvents(ctx, runID)
	if err != nil {
		return nil, false, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行事件失败", err)
	}
	filtered := make([]StreamEventItem, 0, len(rows))
	last := strings.TrimSpace(lastID)
	latest := last
	for _, row := range rows {
		id := row.EventID
		if id == "" {
			id = StreamIDFromSeq(row.Seq)
		}
		if last != "" && last != "0-0" && !isNewerStreamID(id, last) {
			continue
		}
		filtered = append(filtered, StreamEventItem{ID: id, Event: row.EventType, Data: decodePayload(row.PayloadJSON)})
		if latest == "" || isNewerStreamID(id, latest) {
			latest = id
		}
	}
	if latest == "" {
		latest = "0-0"
	}
	errorMsg := ""
	if run.ErrorMsg != nil {
		errorMsg = *run.ErrorMsg
	}
	terminal := isTerminal(run.RunStatus)
	return &EventsResponse{Events: filtered, LastID: latest, RunStatus: run.RunStatus, Terminal: terminal, ErrorMsg: errorMsg}, terminal, nil
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
	record, err := repo.RunForExecute(ctx, runID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI运行失败", err)
	}
	if record != nil && strings.TrimSpace(record.Run.EngineTaskID) != "" {
		engine, appErr := s.engineForApp(ctx, record.App)
		if appErr != nil {
			return nil, appErr
		}
		if err := engine.StopChat(ctx, platformai.StopChatInput{EngineTaskID: record.Run.EngineTaskID, UserKey: userKey(userID)}); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "停止AI引擎任务失败", err)
		}
	}
	if err := repo.MarkCanceled(ctx, runID, "用户取消"); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "取消AI运行失败", err)
	}
	sink := newPersistentSink(repo, s.publisher, userID, runID, s.now)
	if event, err := BuildCancelEvent(runID); err == nil {
		_ = sink.emitEnvelope(ctx, event, "")
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
	engine, appErr := s.engineForApp(ctx, record.App)
	if appErr != nil {
		msg := appErr.Message
		_ = repo.MarkFailed(ctx, input.RunID, msg)
		sink := newPersistentSink(repo, s.publisher, record.Run.UserID, input.RunID, s.now)
		if event, buildErr := BuildFailedEvent(input.RunID, msg); buildErr == nil {
			_ = sink.emitEnvelope(ctx, event, msg)
		}
		return nil, appErr
	}
	start := s.now()
	sink := newPersistentSink(repo, s.publisher, record.Run.UserID, input.RunID, s.now)
	result, err := engine.StreamChat(ctx, platformai.ChatInput{
		AppID:                record.App.AppID,
		RunID:                uint64(input.RunID),
		UserID:               uint64(record.Run.UserID),
		UserKey:              userKey(record.Run.UserID),
		Content:              record.UserMessageContent,
		ConversationEngineID: record.App.ConversationEngineID,
		Inputs:               decodeStringMap(record.App.RuntimeConfigJSON),
	}, sink)
	if err != nil {
		msg := err.Error()
		_ = repo.MarkFailed(ctx, input.RunID, msg)
		if event, buildErr := BuildFailedEvent(input.RunID, msg); buildErr == nil {
			_ = sink.emitEnvelope(ctx, event, msg)
		}
		return nil, err
	}
	if result == nil {
		result = &platformai.ChatResult{}
	}
	latency := result.LatencyMs
	if latency <= 0 {
		latency = int(s.now().Sub(start).Milliseconds())
	}
	usageJSON := mustJSON(map[string]any{
		"prompt_tokens":     result.PromptTokens,
		"completion_tokens": result.CompletionTokens,
		"total_tokens":      result.TotalTokens,
		"cost":              result.Cost,
		"latency_ms":        latency,
	})
	outputJSON := mustJSON(map[string]any{
		"engine_conversation_id": result.EngineConversationID,
		"engine_message_id":      result.EngineMessageID,
		"engine_task_id":         result.EngineTaskID,
	})
	message, err := repo.MarkSuccess(ctx, RunSuccessRecord{
		RunID:                input.RunID,
		ConversationID:       record.Run.ConversationID,
		Content:              result.Answer,
		EngineConversationID: result.EngineConversationID,
		EngineMessageID:      result.EngineMessageID,
		EngineTaskID:         result.EngineTaskID,
		EngineRunID:          result.EngineTaskID,
		UsageJSON:            usageJSON,
		OutputSnapshotJSON:   outputJSON,
		ModelSnapshot:        record.App.ModelSnapshotJSON,
		PromptTokens:         result.PromptTokens,
		CompletionTokens:     result.CompletionTokens,
		TotalTokens:          result.TotalTokens,
		Cost:                 result.Cost,
		LatencyMS:            latency,
	})
	if err != nil {
		return nil, err
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
		_ = sink.emitEnvelope(ctx, completed, "")
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

func (s *Service) resolveAppForCreate(ctx context.Context, repo Repository, userID int64, input *CreateRunInput) (*AppEngineConfig, *apperror.Error) {
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
		if conversation.Status != enum.CommonYes {
			return nil, apperror.BadRequest("AI会话已禁用")
		}
		appID := conversation.AppID
		if appID == 0 {
			appID = uint64(conversation.AgentID)
		}
		if input.AppID > 0 && input.AppID != appID {
			return nil, apperror.BadRequest("会话AI应用不匹配")
		}
		if input.AgentID > 0 && uint64(input.AgentID) != appID {
			return nil, apperror.BadRequest("会话智能体不匹配")
		}
		input.AppID = appID
		input.AgentID = int64(appID)
		app, err := repo.AppForRuntime(ctx, appID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
		}
		if app == nil {
			return nil, apperror.BadRequest("AI应用不存在或已禁用")
		}
		return app, nil
	}
	appID := input.AppID
	if appID == 0 && input.AgentID > 0 {
		appID = uint64(input.AgentID)
	}
	var app *AppEngineConfig
	var err error
	if appID > 0 {
		app, err = repo.AppForRuntime(ctx, appID)
	} else {
		app, err = repo.DefaultActiveApp(ctx)
	}
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if app == nil {
		return nil, apperror.BadRequest("请先配置 AI 应用和 Dify 连接")
	}
	input.AppID = app.AppID
	input.AgentID = int64(app.AppID)
	return app, nil
}

func (s *Service) engineForApp(ctx context.Context, app AppEngineConfig) (platformai.Engine, *apperror.Error) {
	if app.AppID == 0 || app.EngineConnectionID == 0 {
		return nil, apperror.BadRequest("AI应用或供应商未配置")
	}
	if strings.TrimSpace(app.EngineAppAPIKeyEnc) == "" && strings.TrimSpace(app.EngineAPIKeyEnc) == "" {
		return nil, apperror.BadRequest("AI应用API Key未配置")
	}
	apiKeyEnc := strings.TrimSpace(app.EngineAppAPIKeyEnc)
	if apiKeyEnc == "" {
		apiKeyEnc = strings.TrimSpace(app.EngineAPIKeyEnc)
	}
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI应用API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI应用API Key未配置")
	}
	factory := s.engineFactory
	if factory == nil {
		return nil, apperror.Internal("AI引擎工厂未配置")
	}
	engine, err := factory.NewEngine(ctx, EngineConfig{EngineType: platformai.EngineType(app.EngineType), BaseURL: app.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI引擎失败", err)
	}
	return engine, nil
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

func isTerminal(status int) bool {
	return status == enum.AIRunStatusSuccess || status == enum.AIRunStatusFail || status == enum.AIRunStatusCanceled
}

type persistentSink struct {
	repository Repository
	publisher  platformrealtime.Publisher
	userID     int64
	runID      int64
	now        func() time.Time
	mu         sync.Mutex
	seq        uint64
}

func newPersistentSink(repository Repository, publisher platformrealtime.Publisher, userID int64, runID int64, now func() time.Time) *persistentSink {
	if now == nil {
		now = time.Now
	}
	return &persistentSink{repository: repository, publisher: publisher, userID: userID, runID: runID, now: now}
}

func (s *persistentSink) Emit(ctx context.Context, event platformai.Event) error {
	switch event.Type {
	case "start":
		payload := event.Payload
		if payload == nil {
			payload = map[string]any{"run_id": s.runID}
		}
		envelope, err := BuildEventFromPayload(EventAIResponseStart, payload)
		if err != nil {
			return err
		}
		return s.emitEnvelope(ctx, envelope, event.DeltaText)
	case "delta":
		envelope, err := BuildDeltaEvent(s.runID, event.DeltaText)
		if err != nil {
			return err
		}
		return s.emitEnvelope(ctx, envelope, event.DeltaText)
	case "completed":
		return nil
	case "failed":
		msg := engineEventMessage(event)
		envelope, err := BuildFailedEvent(s.runID, msg)
		if err != nil {
			return err
		}
		return s.emitEnvelope(ctx, envelope, msg)
	default:
		payload := event.Payload
		if payload == nil {
			payload = map[string]any{"run_id": s.runID}
		}
		envelope, err := BuildEventFromPayload(event.Type, payload)
		if err != nil {
			return err
		}
		return s.emitEnvelope(ctx, envelope, event.DeltaText)
	}
}

func (s *persistentSink) emitEnvelope(ctx context.Context, event EnvelopeEvent, delta string) error {
	if s == nil || s.repository == nil {
		return nil
	}
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()
	payload := eventPayloadMap(event)
	if delta != "" {
		payload["delta"] = delta
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := s.repository.AppendRunEvent(ctx, RunEventRecord{
		RunID:       s.runID,
		Seq:         seq,
		EventID:     event.ID,
		EventType:   event.Event,
		DeltaText:   delta,
		PayloadJSON: raw,
		CreatedAt:   s.now(),
	}); err != nil {
		return err
	}
	if s.publisher != nil && s.userID > 0 {
		_ = s.publisher.Publish(ctx, platformrealtime.Publication{Platform: enum.PlatformAdmin, UserID: s.userID, Envelope: event.Envelope})
	}
	return nil
}

func decodePayload(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func decodeStringMap(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func userKey(userID int64) string {
	return fmt.Sprintf("admin:%d", userID)
}

func engineEventMessage(event platformai.Event) string {
	if event.Payload != nil {
		if value, ok := event.Payload["message"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := event.Payload["code"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if strings.TrimSpace(event.DeltaText) != "" {
		return strings.TrimSpace(event.DeltaText)
	}
	return "AI运行失败"
}
