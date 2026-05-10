package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
)

type fakeRepository struct {
	conversation *Conversation
	agent        *AgentEngineConfig
	history      []MessageHistory
	assistant    AssistantMessageRecord
	message      Message
	createdRun   CreateRunRecord
	completedRun CompleteRunRecord
	finishedRun  FinishRunRecord
	timeoutLimit int
}

func (f *fakeRepository) ConversationForReply(ctx context.Context, id int64, userID int64) (*Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepository) AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error) {
	return f.agent, nil
}
func (f *fakeRepository) LatestMessages(ctx context.Context, conversationID int64, limit int) ([]MessageHistory, error) {
	return f.history, nil
}
func (f *fakeRepository) InsertAssistantMessage(ctx context.Context, input AssistantMessageRecord) (int64, error) {
	f.assistant = input
	return 22, nil
}
func (f *fakeRepository) CreateRun(ctx context.Context, input CreateRunRecord) (int64, error) {
	f.createdRun = input
	return 100, nil
}
func (f *fakeRepository) CompleteRun(ctx context.Context, input CompleteRunRecord) error {
	f.completedRun = input
	return nil
}
func (f *fakeRepository) FinishRun(ctx context.Context, input FinishRunRecord) error {
	f.finishedRun = input
	return nil
}
func (f *fakeRepository) TimeoutRuns(ctx context.Context, limit int, message string) (int64, error) {
	f.timeoutLimit = limit
	return 2, nil
}

type fakePublisher struct {
	pubs []platformrealtime.Publication
}

func (f *fakePublisher) Publish(ctx context.Context, p platformrealtime.Publication) error {
	f.pubs = append(f.pubs, p)
	return nil
}

type fakeEngineFactory struct {
	engine platformai.Engine
	input  EngineConfig
	err    error
}

func (f *fakeEngineFactory) NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.engine, nil
}

type blankEngine struct {
	platformai.FakeEngine
}

func (blankEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	return &platformai.ChatResult{}, nil
}

type splitDeltaEngine struct{}

func (splitDeltaEngine) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return &platformai.TestConnectionResult{OK: true}, nil
}

func (splitDeltaEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	for _, delta := range []string{"你", "好"} {
		if err := sink.Emit(ctx, platformai.Event{Type: "delta", DeltaText: delta, Payload: map[string]any{"delta": delta}}); err != nil {
			return nil, err
		}
	}
	return &platformai.ChatResult{Answer: "你好", PromptTokens: 4, CompletionTokens: 8, TotalTokens: 12}, nil
}

func (splitDeltaEngine) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	return nil
}

func (splitDeltaEngine) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	return nil, nil
}

func (splitDeltaEngine) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	return nil, nil
}

type captureEngine struct {
	input platformai.ChatInput
}

func (c *captureEngine) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return &platformai.TestConnectionResult{OK: true}, nil
}

func (c *captureEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	c.input = input
	return &platformai.ChatResult{Answer: "看到了图片"}, nil
}

func (c *captureEngine) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	return nil
}

func (c *captureEngine) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	return nil, nil
}

func (c *captureEngine) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	return nil, nil
}

type canceledEngine struct{}

func (canceledEngine) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return &platformai.TestConnectionResult{OK: true}, nil
}

func (canceledEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	return nil, context.Canceled
}

func (canceledEngine) StopChat(ctx context.Context, input platformai.StopChatInput) error { return nil }

func (canceledEngine) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	return nil, nil
}

func (canceledEngine) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	return nil, nil
}
func validAgentConfig(t *testing.T) (*AgentEngineConfig, secretbox.Box) {
	t.Helper()
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentEngineConfig{
		AgentID:          5,
		AgentName:        "客服",
		ProviderID:       2,
		ModelID:          "gpt-5.4",
		ModelDisplayName: "GPT-5.4",
		ScenesJSON:       `["chat"]`,
		EngineType:       string(platformai.EngineTypeDify),
		EngineBaseURL:    "https://dify.test/v1",
		EngineAPIKeyEnc:  cipher,
		AgentStatus:      enum.CommonYes,
		EngineStatus:     enum.CommonYes,
	}, box
}

func TestExecuteConversationReplyPublishesConversationScopedEventsAndPersistsAssistant(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history: []MessageHistory{
			{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"},
		},
	}
	pub := &fakePublisher{}
	factory := &fakeEngineFactory{engine: platformai.NewFakeEngine("ok")}
	res, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: factory, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "ok" || repo.assistant.ConversationID != 3 {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if factory.input.APIKey != "provider-key" || factory.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected engine config: %#v", factory.input)
	}
	if repo.createdRun.ConversationID != 3 || repo.createdRun.RequestID != "rid" || repo.createdRun.ModelID != "gpt-5.4" || repo.createdRun.ModelDisplayName != "GPT-5.4" {
		t.Fatalf("run was not created correctly: %#v", repo.createdRun)
	}
	if repo.completedRun.RunID != 100 || repo.completedRun.AssistantMessageID != 22 {
		t.Fatalf("run was not completed correctly: %#v", repo.completedRun)
	}
	if len(pub.pubs) < 3 || pub.pubs[0].Envelope.Type != EventAIResponseStart || pub.pubs[1].Envelope.Type != EventAIResponseDelta || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseCompleted {
		t.Fatalf("unexpected publications: %#v", pub.pubs)
	}
	for _, pub := range pub.pubs {
		if pub.UserID != 7 || pub.Platform != enum.PlatformAdmin {
			t.Fatalf("publication not scoped to current admin user: %#v", pub)
		}
	}
}

func TestExecuteConversationReplyPreservesStreamingDeltasFromEngine(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	pub := &fakePublisher{}
	res, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: &fakeEngineFactory{engine: splitDeltaEngine{}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "你好" {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if repo.completedRun.TotalTokens != 12 || repo.completedRun.PromptTokens != 4 || repo.completedRun.CompletionTokens != 8 {
		t.Fatalf("run token usage was not persisted: %#v", repo.completedRun)
	}
	var deltas []string
	for _, pub := range pub.pubs {
		if pub.Envelope.Type != EventAIResponseDelta {
			continue
		}
		var payload DeltaPayload
		if err := json.Unmarshal(pub.Envelope.Data, &payload); err != nil {
			t.Fatalf("unexpected delta payload: %v", err)
		}
		deltas = append(deltas, payload.Delta)
	}
	if len(deltas) != 2 || deltas[0] != "你" || deltas[1] != "好" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestExecuteConversationReplyAllowsImageOnlyUserMessage(t *testing.T) {
	agent, box := validAgentConfig(t)
	meta := `{"attachments":[{"type":"image","url":"https://example.test/a.png","name":"a.png","size":1}]}`
	engine := &captureEngine{}
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "", MetaJSON: &meta}},
	}

	res, err := NewService(Dependencies{Repository: repo, Publisher: &fakePublisher{}, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})

	if err != nil {
		t.Fatalf("image-only user message must not be treated as missing: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "看到了图片" {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	attachments, ok := engine.input.Inputs["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("image attachments not passed to engine: %#v", engine.input.Inputs)
	}
	if engine.input.Content != "" {
		t.Fatalf("image-only message should keep empty text content, got %q", engine.input.Content)
	}
}

func TestAssistantMessageZeroValueMetaJSONIsNil(t *testing.T) {
	message := Message{ConversationID: 3, Role: enum.AIMessageRoleAssistant, ContentType: "text", Content: "ok", IsDel: enum.CommonNo}
	if message.MetaJSON != nil {
		t.Fatalf("assistant message without metadata must keep meta_json nil, got %#v", message.MetaJSON)
	}
}

func TestExecuteConversationReplyPublishesFailedForEngineError(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}}}
	pub := &fakePublisher{}
	_, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: &fakeEngineFactory{engine: &platformai.FakeEngine{Err: errors.New("engine down")}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected engine error")
	}
	if len(pub.pubs) == 0 || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseFailed {
		t.Fatalf("expected failed publication, got %#v", pub.pubs)
	}
	if repo.finishedRun.Status != enum.AIRunStatusFailed || repo.finishedRun.Message == "" {
		t.Fatalf("run failure not persisted: %#v", repo.finishedRun)
	}
}

func TestExecuteConversationReplyMarksRunCanceledForCanceledContext(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}}}
	pub := &fakePublisher{}
	_, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: &fakeEngineFactory{engine: canceledEngine{}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if repo.finishedRun.Status != enum.AIRunStatusCanceled {
		t.Fatalf("run cancellation not persisted: %#v", repo.finishedRun)
	}
}
func TestExecuteConversationReplyPublishesFallbackDeltaWhenEngineReturnsBlank(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}},
	}
	pub := &fakePublisher{}
	res, err := NewService(Dependencies{
		Repository:    repo,
		Publisher:     pub,
		EngineFactory: &fakeEngineFactory{engine: &blankEngine{}},
		Secretbox:     box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "AI没有返回内容" {
		t.Fatalf("unexpected fallback result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if len(pub.pubs) < 3 || pub.pubs[len(pub.pubs)-2].Envelope.Type != EventAIResponseDelta || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseCompleted {
		t.Fatalf("expected fallback delta before completed, got %#v", pub.pubs)
	}
}

func TestTimeoutRunsMarksOldRunsFailed(t *testing.T) {
	repo := &fakeRepository{}
	res, err := NewService(Dependencies{Repository: repo}).TimeoutRuns(context.Background(), RunTimeoutInput{Limit: 5})
	if err != nil {
		t.Fatalf("TimeoutRuns returned error: %v", err)
	}
	if repo.timeoutLimit != 5 || res.Failed != 2 {
		t.Fatalf("unexpected timeout result: repo=%#v res=%#v", repo, res)
	}
}

func TestChatHistoryExcludesCurrentUserMessageAndKeepsOrder(t *testing.T) {
	now := time.Now()
	history := chatHistory([]MessageHistory{
		{ID: 3, Role: enum.AIMessageRoleAssistant, Content: "two", CreatedAt: now},
		{ID: 1, Role: enum.AIMessageRoleUser, Content: "one", CreatedAt: now},
		{ID: 4, Role: enum.AIMessageRoleUser, Content: "current", CreatedAt: now},
	}, 4)
	if len(history) != 2 || history[0]["content"] != "one" || history[1]["content"] != "two" {
		t.Fatalf("unexpected history: %#v", history)
	}
}

func TestChatInputsExtractsAttachmentsAndRuntimeParamsFromCurrentMessageMeta(t *testing.T) {
	meta := `{"attachments":[{"type":"image","url":"https://example.test/a.png"}],"runtime_params":{"temperature":0.7,"max_tokens":1024,"max_history":1}}`
	inputs := chatInputs(AgentEngineConfig{ModelID: "gpt-test"}, []MessageHistory{
		{ID: 1, Role: enum.AIMessageRoleUser, Content: "old"},
		{ID: 2, Role: enum.AIMessageRoleAssistant, Content: "older"},
		{ID: 3, Role: enum.AIMessageRoleUser, Content: "current", MetaJSON: &meta},
	}, 3)
	if inputs["temperature"] != 0.7 || inputs["max_tokens"] != 1024.0 {
		t.Fatalf("runtime params not extracted: %#v", inputs)
	}
	attachments, ok := inputs["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachments not extracted: %#v", inputs["attachments"])
	}
	history, ok := inputs["history"].([]map[string]string)
	if !ok || len(history) != 1 || history[0]["content"] != "older" {
		t.Fatalf("max_history not applied: %#v", inputs["history"])
	}
}

func TestChatInputsOnlyIncludesConfiguredSystemPrompt(t *testing.T) {
	inputs := chatInputs(AgentEngineConfig{ModelID: "gpt-test", SystemPrompt: "  "}, nil, 9)
	if _, ok := inputs["system_prompt"]; ok {
		t.Fatalf("blank system prompt must not be sent downstream: %#v", inputs)
	}

	inputs = chatInputs(AgentEngineConfig{ModelID: "gpt-test", SystemPrompt: "你是客服"}, nil, 9)
	if inputs["system_prompt"] != "你是客服" {
		t.Fatalf("configured system prompt must be preserved, got %#v", inputs["system_prompt"])
	}
}

type fakeToolRuntime struct {
	runtimeTools []RuntimeTool
	executeInput []ToolExecuteInput
	executeErr   error
	executeReply map[string]any
}

func (f *fakeToolRuntime) ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeTool, *apperror.Error) {
	return f.runtimeTools, nil
}

func (f *fakeToolRuntime) Execute(ctx context.Context, input ToolExecuteInput) (*ToolExecuteResult, *apperror.Error) {
	f.executeInput = append(f.executeInput, input)
	if f.executeErr != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, f.executeErr.Error(), f.executeErr)
	}
	output := f.executeReply
	if output == nil {
		output = map[string]any{"total_users": 1015, "enabled_users": 1015, "disabled_users": 0}
	}
	raw, _ := json.Marshal(output)
	return &ToolExecuteResult{CallID: input.CallID, Name: input.Tool.Code, Output: raw}, nil
}

type fakeKnowledgeRuntime struct {
	input  KnowledgeRuntimeInput
	result *KnowledgeContextResult
	err    *apperror.Error
}

func (f *fakeKnowledgeRuntime) RetrieveForRun(ctx context.Context, input KnowledgeRuntimeInput) (*KnowledgeContextResult, *apperror.Error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestExecuteConversationReplyInjectsKnowledgeContext(t *testing.T) {
	agent, box := validAgentConfig(t)
	engine := &captureEngine{}
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "这个项目后端架构是什么？"}},
	}
	knowledge := &fakeKnowledgeRuntime{result: &KnowledgeContextResult{RetrievalID: 88, Context: "[K1] 知识库：架构库；文档：Go 后端架构\nGin modular monolith"}}
	_, err := NewService(Dependencies{Repository: repo, Publisher: &fakePublisher{}, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box, KnowledgeRuntime: knowledge}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if knowledge.input.RunID != 100 || knowledge.input.AgentID != 5 || knowledge.input.ConversationID != 3 || knowledge.input.UserMessageID != 9 || knowledge.input.Query != "这个项目后端架构是什么？" {
		t.Fatalf("knowledge runtime input mismatch: %#v", knowledge.input)
	}
	if !strings.Contains(engine.input.Content, "Gin modular monolith") || !strings.Contains(engine.input.Content, "用户问题：\n这个项目后端架构是什么？") {
		t.Fatalf("knowledge context not injected into model input: %q", engine.input.Content)
	}
}

func TestExecuteConversationReplyContinuesWhenKnowledgeRetrievalFails(t *testing.T) {
	agent, box := validAgentConfig(t)
	engine := &captureEngine{}
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}},
	}
	knowledge := &fakeKnowledgeRuntime{err: apperror.Internal("知识库检索失败")}
	res, err := NewService(Dependencies{Repository: repo, Publisher: &fakePublisher{}, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box, KnowledgeRuntime: knowledge}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("knowledge failure must not block chat: %v", err)
	}
	if res.AssistantMessageID != 22 || engine.input.Content != "hi" {
		t.Fatalf("chat should continue with original message: res=%#v input=%q", res, engine.input.Content)
	}
}

type toolCallEngine struct {
	calls  []platformai.ChatInput
	stages int
}

func (e *toolCallEngine) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return &platformai.TestConnectionResult{OK: true}, nil
}

func (e *toolCallEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	e.calls = append(e.calls, input)
	if len(input.ToolOutputs) == 0 {
		return &platformai.ChatResult{ToolCalls: []platformai.ToolCall{{ID: "call-1", Name: "admin_user_count", Arguments: "{}"}}, PromptTokens: 7, CompletionTokens: 1, TotalTokens: 8}, nil
	}
	return &platformai.ChatResult{Answer: "当前用户量1015", PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}, nil
}

func (e *toolCallEngine) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	return nil
}
func (e *toolCallEngine) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	return nil, nil
}
func (e *toolCallEngine) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	return nil, nil
}

func TestExecuteConversationReplySupportsSingleToolRound(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "查用户量"}},
	}
	pub := &fakePublisher{}
	runtime := &fakeToolRuntime{runtimeTools: []RuntimeTool{{ID: 1, Name: "查询当前用户量", Code: "admin_user_count", Description: "查询后台当前用户数量，只返回数量。", ParametersJSON: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, RiskLevel: "low", TimeoutMS: 3000}}}
	engine := &toolCallEngine{}
	res, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box, ToolRuntime: runtime}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "当前用户量1015" {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if len(engine.calls) != 2 {
		t.Fatalf("expected two model calls, got %#v", engine.calls)
	}
	if len(engine.calls[0].Tools) != 1 || engine.calls[0].Tools[0].Name != "admin_user_count" {
		t.Fatalf("runtime tool not passed to engine: %#v", engine.calls[0].Tools)
	}
	if len(engine.calls[1].ToolOutputs) != 1 || engine.calls[1].ToolOutputs[0].CallID != "call-1" {
		t.Fatalf("tool output not passed back to engine: %#v", engine.calls[1].ToolOutputs)
	}
	if len(engine.calls[1].ToolCalls) != 1 || engine.calls[1].ToolCalls[0].ID != "call-1" {
		t.Fatalf("preceding tool call not preserved for second model request: %#v", engine.calls[1].ToolCalls)
	}
	if len(runtime.executeInput) != 1 || runtime.executeInput[0].Tool.Code != "admin_user_count" {
		t.Fatalf("tool runtime not executed: %#v", runtime.executeInput)
	}
	if repo.completedRun.TotalTokens != 13 || repo.completedRun.PromptTokens != 9 || repo.completedRun.CompletionTokens != 4 {
		t.Fatalf("tool round token usage must include both model requests: %#v", repo.completedRun)
	}
}

func TestExecuteConversationReplyRejectsSecondToolRound(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "查用户量"}}}
	runtime := &fakeToolRuntime{runtimeTools: []RuntimeTool{{ID: 1, Name: "查询当前用户量", Code: "admin_user_count", Description: "查询后台当前用户数量，只返回数量。", ParametersJSON: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, RiskLevel: "low", TimeoutMS: 3000}}}
	service := NewService(Dependencies{Repository: repo, Publisher: &fakePublisher{}, EngineFactory: &fakeEngineFactory{engine: &doubleToolRoundEngine{}}, Secretbox: box, ToolRuntime: runtime})
	_, err := service.ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected second round error")
	}
}

type doubleToolRoundEngine struct{}

func (doubleToolRoundEngine) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return &platformai.TestConnectionResult{OK: true}, nil
}
func (doubleToolRoundEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	return &platformai.ChatResult{ToolCalls: []platformai.ToolCall{{ID: "call-1", Name: "admin_user_count", Arguments: "{}"}}}, nil
}
func (doubleToolRoundEngine) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	return nil
}
func (doubleToolRoundEngine) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	return nil, nil
}
func (doubleToolRoundEngine) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	return nil, nil
}
