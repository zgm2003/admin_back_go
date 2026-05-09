package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	platformai "admin_back_go/internal/platform/ai"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
	"admin_back_go/internal/platform/taskqueue"

	"github.com/gin-gonic/gin"
)

type fakeRepository struct {
	activeAgents map[int64]bool
	conversation *Conversation
	run          *Run
	runSequence  []*Run
	runCalls     int
	agent        *AgentEngineConfig
	events       []RunEventRecord
	userMessage  *Message
	assistant    *Message
	createdInput CreateRunRecord
	createdRunID int64
	cancelID     int64
	successID    int64
	failedID     int64
	failedMsg    string
	timeoutLimit int
}

func (f *fakeRepository) ActiveAgentExists(ctx context.Context, id int64) (bool, error) {
	return f.activeAgents[id], nil
}
func (f *fakeRepository) DefaultActiveAgent(ctx context.Context) (*AgentEngineConfig, error) {
	return f.agent, nil
}
func (f *fakeRepository) AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error) {
	if f.agent == nil {
		return nil, nil
	}
	return f.agent, nil
}
func (f *fakeRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepository) CreateRun(ctx context.Context, input CreateRunRecord) (*RunStartRecord, error) {
	f.createdInput = input
	if f.createdRunID == 0 {
		f.createdRunID = 11
	}
	return &RunStartRecord{RunID: f.createdRunID, ConversationID: 3, RequestID: "rid", UserMessageID: 9, AgentID: input.AgentID, IsNew: input.ConversationID == 0}, nil
}
func (f *fakeRepository) RunForUser(ctx context.Context, runID int64, userID int64) (*Run, error) {
	f.runCalls++
	if len(f.runSequence) > 0 {
		index := f.runCalls - 1
		if index >= len(f.runSequence) {
			index = len(f.runSequence) - 1
		}
		if f.runCalls >= 2 && len(f.events) == 0 {
			f.events = []RunEventRecord{{RunID: 8, Seq: 2, EventID: "2-0", EventType: EventAIResponseDelta, DeltaText: "done", PayloadJSON: json.RawMessage(`{"run_id":8,"delta":"done"}`)}}
		}
		return f.runSequence[index], nil
	}
	return f.run, nil
}
func (f *fakeRepository) RunForExecute(ctx context.Context, runID int64) (*RunExecutionRecord, error) {
	if f.run == nil {
		return nil, nil
	}
	content := "hi"
	if f.userMessage != nil {
		content = f.userMessage.Content
	}
	record := &RunExecutionRecord{Run: *f.run, UserMessageContent: content}
	if f.agent != nil {
		record.Agent = *f.agent
	}
	return record, nil
}
func (f *fakeRepository) AssistantMessage(ctx context.Context, id int64) (*Message, error) {
	return f.assistant, nil
}
func (f *fakeRepository) MarkCanceled(ctx context.Context, runID int64, message string) error {
	f.cancelID = runID
	return nil
}
func (f *fakeRepository) MarkSuccess(ctx context.Context, input RunSuccessRecord) (*Message, error) {
	f.successID = input.RunID
	msg := &Message{ID: 22, ConversationID: input.ConversationID, Role: enum.AIMessageRoleAssistant, Content: input.Content}
	f.assistant = msg
	return msg, nil
}
func (f *fakeRepository) MarkFailed(ctx context.Context, runID int64, message string) error {
	f.failedID = runID
	f.failedMsg = message
	return nil
}
func (f *fakeRepository) AppendRunEvent(ctx context.Context, input RunEventRecord) error {
	f.events = append(f.events, input)
	return nil
}
func (f *fakeRepository) ListRunEvents(ctx context.Context, runID int64) ([]RunEventRecord, error) {
	return f.events, nil
}
func (f *fakeRepository) TimeoutRuns(ctx context.Context, limit int, message string) (int64, error) {
	f.timeoutLimit = limit
	return 2, nil
}

type fakeEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	f.tasks = append(f.tasks, task)
	return taskqueue.EnqueueResult{ID: "task-id", Type: task.Type, Queue: task.Queue}, f.err
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
	err    error
	input  EngineConfig
}

func (f *fakeEngineFactory) NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	if f.engine == nil {
		return nil, platformai.ErrInvalidConfig
	}
	return f.engine, nil
}

func validAgentConfig(t *testing.T) (*AgentEngineConfig, secretbox.Box) {
	t.Helper()
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("agent-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentEngineConfig{
		AgentID:                5,
		AgentName:              "客服",
		ProviderID:             2,
		EngineType:             string(platformai.EngineTypeDify),
		EngineBaseURL:          "https://dify.test/v1",
		ExternalAgentAPIKeyEnc: cipher,
		RuntimeConfigJSON:      `{"tenant":"admin"}`,
		ModelSnapshotJSON:      `{"provider":"dify"}`,
		AgentStatus:            enum.CommonYes,
		EngineStatus:           enum.CommonYes,
	}, box
}

func TestCreateRunCreatesConversationMessageRunEnqueuesAndPublishesStart(t *testing.T) {
	agent, _ := validAgentConfig(t)
	repo := &fakeRepository{activeAgents: map[int64]bool{5: true}, agent: agent}
	enq := &fakeEnqueuer{}
	pub := &fakePublisher{}
	res, appErr := NewService(Dependencies{Repository: repo, Enqueuer: enq, Publisher: pub}).CreateRun(context.Background(), 7, CreateRunInput{AgentID: 5, Content: " hello "})
	if appErr != nil {
		t.Fatalf("CreateRun returned error: %v", appErr)
	}
	if res.RunID != 11 || repo.createdInput.UserID != 7 || repo.createdInput.Content != "hello" || repo.createdInput.AgentID != 5 {
		t.Fatalf("unexpected create: res=%#v input=%#v", res, repo.createdInput)
	}
	if len(enq.tasks) != 1 || enq.tasks[0].Type != TypeRunExecuteV1 {
		t.Fatalf("expected run execute task, got %#v", enq.tasks)
	}
	if len(pub.pubs) != 1 || pub.pubs[0].Envelope.Type != EventAIResponseStart || pub.pubs[0].UserID != 7 {
		t.Fatalf("expected start publication, got %#v", pub.pubs)
	}
}

func TestCreateRunUsesExistingConversationAgentWhenAgentIDIsMissing(t *testing.T) {
	agent, _ := validAgentConfig(t)
	repo := &fakeRepository{
		activeAgents: map[int64]bool{5: true},
		agent:          agent,
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, Status: enum.CommonYes, IsDel: enum.CommonNo},
	}
	res, appErr := NewService(Dependencies{Repository: repo}).CreateRun(context.Background(), 7, CreateRunInput{ConversationID: 3, Content: " follow up "})
	if appErr != nil {
		t.Fatalf("CreateRun returned error: %v", appErr)
	}
	if res.AgentID != 5 || repo.createdInput.AgentID != 5 || repo.createdInput.ConversationID != 3 || repo.createdInput.Content != "follow up" {
		t.Fatalf("unexpected existing conversation run: res=%#v input=%#v", res, repo.createdInput)
	}
}

func TestCreateRunHandlerAcceptsConversationIDWithoutAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, SessionID: 1, Platform: "admin"})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-chat/runs", strings.NewReader(`{"content":"follow up","conversation_id":3}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected handler to accept conversation-only run, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.ConversationID != 3 || service.input.AgentID != 0 || service.input.Content != "follow up" {
		t.Fatalf("unexpected service input: %#v", service.input)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 {
		t.Fatalf("expected ok response, got body=%s", recorder.Body.String())
	}
}

func TestCreateRunRejectsMissingContentAndInactiveAgent(t *testing.T) {
	repo := &fakeRepository{activeAgents: map[int64]bool{5: false}}
	_, appErr := NewService(Dependencies{Repository: repo}).CreateRun(context.Background(), 7, CreateRunInput{AgentID: 5, Content: ""})
	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad content, got %#v", appErr)
	}
	_, appErr = NewService(Dependencies{Repository: repo}).CreateRun(context.Background(), 7, CreateRunInput{AgentID: 5, Content: "hello"})
	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad agent, got %#v", appErr)
	}
}

func TestEventsRejectsForeignRunAndReplaysPersistedEvents(t *testing.T) {
	repo := &fakeRepository{
		run: &Run{ID: 8, UserID: 7, RunStatus: enum.AIRunStatusSuccess, ConversationID: 3, UserMessageID: ptrInt64(9), AssistantMessageID: ptrInt64(10), RequestID: "rid"},
		events: []RunEventRecord{
			{RunID: 8, Seq: 1, EventID: "1-0", EventType: EventAIResponseStart, PayloadJSON: json.RawMessage(`{"run_id":8}`)},
			{RunID: 8, Seq: 2, EventID: "2-0", EventType: EventAIResponseDelta, DeltaText: "done", PayloadJSON: json.RawMessage(`{"run_id":8,"delta":"done"}`)},
			{RunID: 8, Seq: 3, EventID: "3-0", EventType: EventAIResponseCompleted, PayloadJSON: json.RawMessage(`{"run_id":8,"conversation_id":3,"user_message_id":9,"assistant_message_id":10}`)},
		},
	}
	res, appErr := NewService(Dependencies{Repository: repo}).Events(context.Background(), 7, 8, "0-0", 0)
	if appErr != nil {
		t.Fatalf("Events returned error: %v", appErr)
	}
	if !res.Terminal || len(res.Events) != 3 || res.Events[len(res.Events)-1].Event != EventAIResponseCompleted {
		t.Fatalf("unexpected events: %#v", res)
	}
	repo.run.UserID = 9
	_, appErr = NewService(Dependencies{Repository: repo}).Events(context.Background(), 7, 8, "0-0", 0)
	if appErr == nil || appErr.Code != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestEventsLongPollWaitsForNewEvent(t *testing.T) {
	repo := &fakeRepository{
		runSequence: []*Run{
			{ID: 8, UserID: 7, RunStatus: enum.AIRunStatusRunning, ConversationID: 3, UserMessageID: ptrInt64(9), RequestID: "rid"},
			{ID: 8, UserID: 7, RunStatus: enum.AIRunStatusSuccess, ConversationID: 3, UserMessageID: ptrInt64(9), AssistantMessageID: ptrInt64(10), RequestID: "rid"},
		},
	}

	res, appErr := NewService(Dependencies{Repository: repo}).Events(context.Background(), 7, 8, "1-0", 300*time.Millisecond)
	if appErr != nil {
		t.Fatalf("Events returned error: %v", appErr)
	}
	if repo.runCalls < 2 {
		t.Fatalf("expected long-poll to recheck run state, calls=%d", repo.runCalls)
	}
	if !res.Terminal || len(res.Events) == 0 {
		t.Fatalf("expected terminal replay event after wait, got %#v", res)
	}
}

func TestCancelOnlyCancelsRunningOwnerRunAndPublishes(t *testing.T) {
	agent, _ := validAgentConfig(t)
	engine := platformai.NewFakeEngine("ok")
	repo := &fakeRepository{run: &Run{ID: 8, UserID: 7, AgentID: 5, EngineTaskID: "task-1", RunStatus: enum.AIRunStatusRunning}, agent: agent}
	pub := &fakePublisher{}
	res, appErr := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: secretbox.New("vault-key")}).Cancel(context.Background(), 7, 8)
	if appErr != nil {
		t.Fatalf("Cancel returned error: %v", appErr)
	}
	if repo.cancelID != 8 || res.Status != "canceled" || len(pub.pubs) != 1 || pub.pubs[0].Envelope.Type != EventAIResponseCancel || len(engine.Stopped) != 1 {
		t.Fatalf("unexpected cancel: res=%#v pubs=%#v stopped=%#v", res, pub.pubs, engine.Stopped)
	}
}

func TestExecuteRunMarksSuccessAndFailure(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{agent: agent, run: &Run{ID: 8, UserID: 7, AgentID: 5, ConversationID: 3, UserMessageID: ptrInt64(9), RunStatus: enum.AIRunStatusRunning}, userMessage: &Message{ID: 9, Content: "hi"}}
	pub := &fakePublisher{}
	_, err := NewService(Dependencies{Repository: repo, EngineFactory: &fakeEngineFactory{engine: platformai.NewFakeEngine("ok")}, Secretbox: box, Publisher: pub}).ExecuteRun(context.Background(), RunExecuteInput{RunID: 8})
	if err != nil {
		t.Fatalf("ExecuteRun returned error: %v", err)
	}
	if repo.successID != 8 || len(pub.pubs) < 3 || pub.pubs[0].Envelope.Type != EventAIResponseStart || pub.pubs[1].Envelope.Type != EventAIResponseDelta || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseCompleted {
		t.Fatalf("unexpected success: repo=%#v pubs=%#v", repo, pub.pubs)
	}

	repo = &fakeRepository{agent: agent, run: &Run{ID: 9, UserID: 7, AgentID: 5, ConversationID: 3, UserMessageID: ptrInt64(9), RunStatus: enum.AIRunStatusRunning}}
	_, err = NewService(Dependencies{Repository: repo, EngineFactory: &fakeEngineFactory{engine: &platformai.FakeEngine{Err: errors.New("engine down")}}, Secretbox: box}).ExecuteRun(context.Background(), RunExecuteInput{RunID: 9})
	if err == nil || repo.failedID != 9 || repo.failedMsg != "engine down" {
		t.Fatalf("expected failure mark, err=%v repo=%#v", err, repo)
	}
}

func TestExecuteRunUsesEngineAndPersistsEvents(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		agent:        agent,
		run:         &Run{ID: 8, UserID: 7, AgentID: 5, ProviderID: 2, ConversationID: 3, UserMessageID: ptrInt64(9), RunStatus: enum.AIRunStatusRunning},
		userMessage: &Message{ID: 9, Content: "hi"},
	}
	pub := &fakePublisher{}
	factory := &fakeEngineFactory{engine: platformai.NewFakeEngine("engine ok")}

	_, err := NewService(Dependencies{Repository: repo, EngineFactory: factory, Secretbox: box, Publisher: pub}).ExecuteRun(context.Background(), RunExecuteInput{RunID: 8})
	if err != nil {
		t.Fatalf("ExecuteRun returned error: %v", err)
	}
	if factory.input.APIKey != "agent-key" || factory.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected engine factory input: %#v", factory.input)
	}
	if repo.successID != 8 || repo.assistant == nil || repo.assistant.Content != "engine ok" {
		t.Fatalf("expected success assistant message, repo=%#v", repo)
	}
	if len(repo.events) < 3 {
		t.Fatalf("expected persisted start/delta/completed events, got %#v", repo.events)
	}
	if len(pub.pubs) < 3 {
		t.Fatalf("expected realtime publications, got %#v", pub.pubs)
	}
}

func TestExecuteRunWithoutEngineConfigFailsExplicitly(t *testing.T) {
	repo := &fakeRepository{run: &Run{ID: 8, UserID: 7, AgentID: 5, ConversationID: 3, UserMessageID: ptrInt64(9), RunStatus: enum.AIRunStatusRunning}}

	_, err := NewService(Dependencies{Repository: repo}).ExecuteRun(context.Background(), RunExecuteInput{RunID: 8})
	if err == nil || repo.failedID != 8 || !strings.Contains(repo.failedMsg, "AI智能体或供应商未配置") {
		t.Fatalf("expected explicit engine config failure, err=%v repo=%#v", err, repo)
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

type fakeHTTPService struct {
	input CreateRunInput
}

func (f *fakeHTTPService) CreateRun(ctx context.Context, userID int64, input CreateRunInput) (*CreateRunResponse, *apperror.Error) {
	f.input = input
	return &CreateRunResponse{ConversationID: input.ConversationID, RunID: 11, RequestID: "rid", UserMessageID: 9, AgentID: input.AgentID}, nil
}

func (f *fakeHTTPService) Events(ctx context.Context, userID int64, runID int64, lastID string, timeout time.Duration) (*EventsResponse, *apperror.Error) {
	return nil, nil
}

func (f *fakeHTTPService) Cancel(ctx context.Context, userID int64, runID int64) (*CancelResponse, *apperror.Error) {
	return nil, nil
}

func ptrInt64(v int64) *int64 { return &v }
