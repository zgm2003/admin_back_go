package aichat

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/enum"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/taskqueue"
)

type fakeRepository struct {
	activeAgents map[int64]bool
	conversation *Conversation
	run          *Run
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
	return &RunExecutionRecord{Run: *f.run, UserMessageContent: content}, nil
}
func (f *fakeRepository) AssistantMessage(ctx context.Context, id int64) (*Message, error) {
	return f.assistant, nil
}
func (f *fakeRepository) MarkCanceled(ctx context.Context, runID int64) error {
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

type fakeProvider struct {
	text string
	err  error
}

func (f fakeProvider) Generate(ctx context.Context, input GenerateInput) (*GenerateResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &GenerateResult{Content: f.text, ModelSnapshot: "fake-model", PromptTokens: 1, CompletionTokens: 2}, nil
}

func TestCreateRunCreatesConversationMessageRunEnqueuesAndPublishesStart(t *testing.T) {
	repo := &fakeRepository{activeAgents: map[int64]bool{5: true}}
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

func TestEventsRejectsForeignRunAndReplaysTerminalEvents(t *testing.T) {
	repo := &fakeRepository{run: &Run{ID: 8, UserID: 7, RunStatus: enum.AIRunStatusSuccess, ConversationID: 3, UserMessageID: ptrInt64(9), AssistantMessageID: ptrInt64(10), RequestID: "rid"}, assistant: &Message{ID: 10, Content: "done"}}
	res, appErr := NewService(Dependencies{Repository: repo}).Events(context.Background(), 7, 8, "0-0")
	if appErr != nil {
		t.Fatalf("Events returned error: %v", appErr)
	}
	if !res.Terminal || len(res.Events) < 3 || res.Events[len(res.Events)-1].Event != EventAIResponseCompleted {
		t.Fatalf("unexpected events: %#v", res)
	}
	repo.run.UserID = 9
	_, appErr = NewService(Dependencies{Repository: repo}).Events(context.Background(), 7, 8, "0-0")
	if appErr == nil || appErr.Code != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestCancelOnlyCancelsRunningOwnerRunAndPublishes(t *testing.T) {
	repo := &fakeRepository{run: &Run{ID: 8, UserID: 7, RunStatus: enum.AIRunStatusRunning}}
	pub := &fakePublisher{}
	res, appErr := NewService(Dependencies{Repository: repo, Publisher: pub}).Cancel(context.Background(), 7, 8)
	if appErr != nil {
		t.Fatalf("Cancel returned error: %v", appErr)
	}
	if repo.cancelID != 8 || res.Status != "canceled" || len(pub.pubs) != 1 || pub.pubs[0].Envelope.Type != EventAIResponseCancel {
		t.Fatalf("unexpected cancel: res=%#v pubs=%#v", res, pub.pubs)
	}
}

func TestExecuteRunMarksSuccessAndFailure(t *testing.T) {
	repo := &fakeRepository{run: &Run{ID: 8, UserID: 7, AgentID: 5, ConversationID: 3, UserMessageID: ptrInt64(9), RunStatus: enum.AIRunStatusRunning}, userMessage: &Message{ID: 9, Content: "hi"}}
	pub := &fakePublisher{}
	_, err := NewService(Dependencies{Repository: repo, Provider: fakeProvider{text: "ok"}, Publisher: pub}).ExecuteRun(context.Background(), RunExecuteInput{RunID: 8})
	if err != nil {
		t.Fatalf("ExecuteRun returned error: %v", err)
	}
	if repo.successID != 8 || len(pub.pubs) != 2 || pub.pubs[0].Envelope.Type != EventAIResponseDelta || pub.pubs[1].Envelope.Type != EventAIResponseCompleted {
		t.Fatalf("unexpected success: repo=%#v pubs=%#v", repo, pub.pubs)
	}

	repo = &fakeRepository{run: &Run{ID: 9, UserID: 7, AgentID: 5, ConversationID: 3, UserMessageID: ptrInt64(9), RunStatus: enum.AIRunStatusRunning}}
	_, err = NewService(Dependencies{Repository: repo, Provider: fakeProvider{err: errors.New("provider down")}}).ExecuteRun(context.Background(), RunExecuteInput{RunID: 9})
	if err == nil || repo.failedID != 9 || repo.failedMsg != "provider down" {
		t.Fatalf("expected failure mark, err=%v repo=%#v", err, repo)
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

func ptrInt64(v int64) *int64 { return &v }
