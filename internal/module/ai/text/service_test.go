package aitext

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/enum"

	"github.com/hibiken/asynq"
)

type fakeDurableStore struct {
	mu            sync.Mutex
	tasks         map[string]*TextTask
	replays       map[string]*ReplayRecord
	acceptCalls   int
	replayCalls   int
	findCalls     int
	conflict      bool
	terminal      *TextTask
	accepted      chan struct{}
	findStarted   chan struct{}
	findStartOnce sync.Once
	acceptCtxErr  error
}

func (f *fakeDurableStore) Accept(ctx context.Context, input AcceptInput) (*TextTask, error) {
	return f.accept(ctx, input)
}

func (f *fakeDurableStore) accept(ctx context.Context, input AcceptInput) (*TextTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acceptCtxErr = ctx.Err()
	f.acceptCalls++
	if f.accepted != nil {
		select {
		case <-f.accepted:
		default:
			close(f.accepted)
		}
	}
	if f.conflict {
		return nil, requestidentity.ErrRequestIdentityConflict
	}
	if f.tasks == nil {
		f.tasks = map[string]*TextTask{}
	}
	key := input.RequestID
	if existing := f.tasks[key]; existing != nil {
		var stored [32]byte
		copy(stored[:], existing.RequestFingerprint)
		if stored != input.RequestFingerprint {
			return nil, requestidentity.ErrRequestIdentityConflict
		}
		copy := *existing
		return &copy, nil
	}
	task := &TextTask{
		ID:                    41,
		Platform:              input.Platform,
		UserID:                input.UserID,
		RequestID:             input.RequestID,
		RequestFingerprint:    append([]byte(nil), input.RequestFingerprint[:]...),
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
		RunID:                 51,
		Kind:                  input.Kind,
		Status:                StatusRunning,
	}
	f.tasks[key] = task
	copy := *task
	return &copy, nil
}

func (f *fakeDurableStore) FindByID(ctx context.Context, taskID uint64) (*TextTask, error) {
	f.mu.Lock()
	f.findCalls++
	terminal := f.terminal
	f.mu.Unlock()
	if f.findStarted != nil {
		f.findStartOnce.Do(func() { close(f.findStarted) })
	}
	if terminal != nil {
		copy := *terminal
		return &copy, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeDurableStore) FindReplay(_ context.Context, userID int64, requestID string) (*ReplayRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replayCalls++
	replay := f.replays[requestID]
	if replay == nil || replay.Task.UserID != userID {
		return nil, nil
	}
	copy := *replay
	copy.Task = *cloneTask(&replay.Task)
	return &copy, nil
}

func (f *fakeDurableStore) LoadExecution(context.Context, uint64) (*Execution, error) {
	return nil, errors.New("not used")
}

func (f *fakeDurableStore) SaveCandidate(context.Context, uint64, int64, string) error {
	return errors.New("not used")
}

func (f *fakeDurableStore) MarkExecutionFailure(context.Context, uint64, int64, string, string) error {
	return errors.New("not used")
}

type fakeWaker struct {
	mu       sync.Mutex
	calls    int
	taskID   uint64
	ctxErr   error
	woke     chan struct{}
	wakeOnce sync.Once
}

func (f *fakeWaker) WakeTextTask(ctx context.Context, taskID uint64) error {
	f.mu.Lock()
	f.calls++
	f.taskID = taskID
	f.ctxErr = ctx.Err()
	f.mu.Unlock()
	if f.woke != nil {
		f.wakeOnce.Do(func() { close(f.woke) })
	}
	return nil
}

func testAcceptInput(requestID string) AcceptInput {
	fingerprint, _ := requestidentity.BuildFingerprint(requestidentity.Input{
		UserID: 7, Operation: "tool.generate_draft", Modality: "text", AgentID: 5,
		ModelID: "gpt-5.4", NormalizedText: `{"requirement":"count users"}`,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: 64},
	})
	return AcceptInput{
		Platform: enum.PlatformAdmin, UserID: 7, RequestID: requestID, RequestFingerprint: fingerprint,
		Kind: KindToolDraft, AgentID: 5, ProviderID: 2, ModelID: "gpt-5.4",
		ModelDisplayName: "GPT 5.4", Prompt: "count users", InputSnapshot: `{"prompt":"count users"}`,
		PricingSnapshotJSON: `{"version":"test-v1"}`, EffectiveMaxOutputTokens: 64,
	}
}

func TestSubmitRejectsBlankRequestIDBeforeAccept(t *testing.T) {
	store := &fakeDurableStore{}
	service := NewService(ServiceDependencies{Store: store, Waker: &fakeWaker{}})

	_, appErr := service.Submit(context.Background(), testAcceptInput("   "))

	if appErr == nil || appErr.Code != ErrorCodeRequestInvalid {
		t.Fatalf("blank request_id error = %#v", appErr)
	}
	if store.acceptCalls != 0 {
		t.Fatalf("store called for blank request_id: %d", store.acceptCalls)
	}
}

func TestSubmitReplaysSameFingerprintAndRejectsConflictBeforeWake(t *testing.T) {
	store := &fakeDurableStore{}
	waker := &fakeWaker{}
	service := NewService(ServiceDependencies{Store: store, Waker: waker})
	input := testAcceptInput("request-1")

	first, appErr := service.Submit(context.Background(), input)
	if appErr != nil {
		t.Fatalf("first Submit error: %v", appErr)
	}
	second, appErr := service.Submit(context.Background(), input)
	if appErr != nil {
		t.Fatalf("replay Submit error: %v", appErr)
	}
	if first.ID != second.ID || first.RunID != second.RunID {
		t.Fatalf("replay identity changed: first=%#v second=%#v", first, second)
	}

	conflict := input
	conflict.RequestFingerprint[0] ^= 0xff
	_, appErr = service.Submit(context.Background(), conflict)
	if appErr == nil || appErr.Code != requestidentity.ErrorCodeFingerprintConflict || appErr.HTTPStatus != 409 {
		t.Fatalf("fingerprint conflict error = %#v", appErr)
	}
	waker.mu.Lock()
	calls := waker.calls
	waker.mu.Unlock()
	if calls != 2 {
		t.Fatalf("conflicting request woke worker: wake calls=%d", calls)
	}
}

func TestReplayAndWaitUsesPersistedModelAndMaxOutputBeforeAcceptance(t *testing.T) {
	snapshot, err := EncodeProviderInputSnapshot(ProviderInputSnapshot{
		Operation: "tool.generate_draft", Modality: "text", NormalizedText: `{"requirement":"count users"}`,
		MaxOutputTokens: 64, Prompt: "count users", SystemPrompt: "strict json",
	})
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	fingerprint, err := requestidentity.BuildFingerprint(requestidentity.Input{
		UserID: 7, Operation: "tool.generate_draft", Modality: "text", AgentID: 5,
		ModelID: "retired-model", NormalizedText: `{"requirement":"count users"}`,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: 64},
	})
	if err != nil {
		t.Fatalf("build fingerprint: %v", err)
	}
	answer := "persisted answer"
	task := TextTask{
		ID: 41, UserID: 7, RequestID: "request-replay", RequestFingerprint: append([]byte(nil), fingerprint[:]...),
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), RunID: 51, Kind: KindToolDraft,
		AgentID: 5, ModelID: "retired-model", Status: StatusSuccess, Answer: &answer,
	}
	store := &fakeDurableStore{
		replays:  map[string]*ReplayRecord{"request-replay": {Task: task, InputSnapshot: snapshot}},
		terminal: &task,
	}
	waker := &fakeWaker{}
	service := NewService(ServiceDependencies{Store: store, Waker: waker})

	result, found, appErr := service.ReplayAndWait(context.Background(), ReplayInput{
		UserID: 7, RequestID: "request-replay", AgentID: 5, Operation: "tool.generate_draft", Modality: "text",
		NormalizedText: `{"requirement":"count users"}`,
	})
	if appErr != nil || !found || result == nil || result.Answer != answer {
		t.Fatalf("replay result=%#v found=%v error=%v", result, found, appErr)
	}
	if store.acceptCalls != 0 {
		t.Fatalf("replay accepted new work: %d", store.acceptCalls)
	}

	_, found, appErr = service.ReplayAndWait(context.Background(), ReplayInput{
		UserID: 7, RequestID: "request-replay", AgentID: 5, Operation: "tool.generate_draft", Modality: "text",
		NormalizedText: `{"requirement":"count orders"}`,
	})
	if appErr == nil || appErr.Code != requestidentity.ErrorCodeFingerprintConflict || !found {
		t.Fatalf("conflicting replay found=%v error=%#v", found, appErr)
	}
	if store.acceptCalls != 0 || waker.calls != 0 {
		t.Fatalf("conflicting replay performed work: accepts=%d wakes=%d", store.acceptCalls, waker.calls)
	}
}

func TestReplayAndWaitWakesPersistedRunningTask(t *testing.T) {
	input := testAcceptInput("request-running")
	snapshot, err := EncodeProviderInputSnapshot(ProviderInputSnapshot{
		Operation: "tool.generate_draft", Modality: "text", NormalizedText: `{"requirement":"count users"}`,
		MaxOutputTokens: 64, Prompt: "count users",
	})
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	running := TextTask{
		ID: 41, UserID: 7, RequestID: input.RequestID, RequestFingerprint: append([]byte(nil), input.RequestFingerprint[:]...),
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), RunID: 51, Kind: KindToolDraft,
		AgentID: 5, ModelID: input.ModelID, Status: StatusRunning,
	}
	answer := "finished answer"
	terminal := running
	terminal.Status = StatusSuccess
	terminal.Answer = &answer
	store := &fakeDurableStore{
		replays:  map[string]*ReplayRecord{input.RequestID: {Task: running, InputSnapshot: snapshot}},
		terminal: &terminal,
	}
	waker := &fakeWaker{}
	service := NewService(ServiceDependencies{Store: store, Waker: waker, WaitInterval: time.Millisecond})

	result, found, appErr := service.ReplayAndWait(context.Background(), ReplayInput{
		UserID: 7, RequestID: input.RequestID, AgentID: 5, Operation: "tool.generate_draft", Modality: "text",
		NormalizedText: `{"requirement":"count users"}`,
	})
	if appErr != nil || !found || result == nil || result.Answer != answer {
		t.Fatalf("running replay result=%#v found=%v error=%v", result, found, appErr)
	}
	if waker.calls != 1 {
		t.Fatalf("running replay wake calls=%d, want 1", waker.calls)
	}
}

func TestSubmitAndWaitHTTPContextCancellationDoesNotCancelWakeContext(t *testing.T) {
	store := &fakeDurableStore{accepted: make(chan struct{}), findStarted: make(chan struct{})}
	waker := &fakeWaker{woke: make(chan struct{})}
	service := NewService(ServiceDependencies{Store: store, Waker: waker, WaitInterval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *struct {
		result *Result
		err    error
	}, 1)
	go func() {
		result, appErr := service.SubmitAndWait(ctx, testAcceptInput("request-cancel"))
		var err error
		if appErr != nil {
			err = appErr
		}
		done <- &struct {
			result *Result
			err    error
		}{result: result, err: err}
	}()

	<-waker.woke
	<-store.findStarted
	cancel()
	out := <-done
	if out.result != nil || !errors.Is(out.err, context.Canceled) {
		t.Fatalf("wait cancellation result=%#v err=%v", out.result, out.err)
	}
	waker.mu.Lock()
	wakeCtxErr := waker.ctxErr
	waker.mu.Unlock()
	if wakeCtxErr != nil {
		t.Fatalf("HTTP cancellation leaked into worker wake context: %v", wakeCtxErr)
	}
}

func TestAlreadyCanceledHTTPContextStillAcceptsAndWakesDurableTask(t *testing.T) {
	store := &fakeDurableStore{}
	waker := &fakeWaker{}
	service := NewService(ServiceDependencies{Store: store, Waker: waker, WaitInterval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, appErr := service.SubmitAndWait(ctx, testAcceptInput("request-already-canceled"))

	if result != nil || appErr == nil || !errors.Is(appErr, context.Canceled) {
		t.Fatalf("result=%#v error=%#v", result, appErr)
	}
	store.mu.Lock()
	acceptCtxErr := store.acceptCtxErr
	acceptCalls := store.acceptCalls
	store.mu.Unlock()
	waker.mu.Lock()
	wakeCtxErr := waker.ctxErr
	wakeCalls := waker.calls
	waker.mu.Unlock()
	if acceptCalls != 1 || wakeCalls != 1 || acceptCtxErr != nil || wakeCtxErr != nil {
		t.Fatalf("accept_calls=%d wake_calls=%d accept_ctx=%v wake_ctx=%v", acceptCalls, wakeCalls, acceptCtxErr, wakeCtxErr)
	}
}

func TestSubmitAndWaitMapsInsufficientBalanceTerminal(t *testing.T) {
	message := "余额不足，请充值后重试"
	store := &fakeDurableStore{terminal: &TextTask{
		ID: 41, UserID: 7, RunID: 51, Status: StatusFailed,
		LastErrorCode: ErrorCodeInsufficientBalance, ErrorMessage: &message,
	}}
	service := NewService(ServiceDependencies{Store: store, Waker: &fakeWaker{}, WaitInterval: time.Millisecond})

	_, appErr := service.SubmitAndWait(context.Background(), testAcceptInput("request-low-balance"))

	if appErr == nil || appErr.Code != ErrorCodeInsufficientBalance || appErr.HTTPStatus != 409 {
		t.Fatalf("insufficient balance error = %#v", appErr)
	}
}

type fakeJobService struct{ taskID uint64 }

func (f *fakeJobService) ExecuteTask(_ context.Context, taskID uint64) error {
	f.taskID = taskID
	return nil
}

func TestTextTaskDefinitionDispatchesDurableTaskID(t *testing.T) {
	job := &fakeJobService{}
	registry := taskqueue.NewRegistry()
	if err := RegisterTaskDefinitions(registry, job, nil); err != nil {
		t.Fatalf("RegisterTaskDefinitions: %v", err)
	}
	mux := taskqueue.NewMux()
	if err := mux.RegisterRegistry(registry); err != nil {
		t.Fatalf("RegisterRegistry: %v", err)
	}
	task, err := NewGenerateTask(GeneratePayload{TaskID: 41})
	if err != nil {
		t.Fatalf("NewGenerateTask: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask: %v", err)
	}
	if job.taskID != 41 {
		t.Fatalf("worker task id = %d", job.taskID)
	}
}

type duplicateTextTaskEnqueuer struct{}

func (duplicateTextTaskEnqueuer) Enqueue(context.Context, taskqueue.Task) (taskqueue.EnqueueResult, error) {
	return taskqueue.EnqueueResult{}, asynq.ErrDuplicateTask
}

func TestWakeTextTaskTreatsDuplicateEnqueueAsAlreadyRunning(t *testing.T) {
	waker := NewWakeupEnqueuer(duplicateTextTaskEnqueuer{})

	if err := waker.WakeTextTask(context.Background(), 41); err != nil {
		t.Fatalf("duplicate durable task wake should wait existing work: %v", err)
	}
}

func TestProviderInputSnapshotRestoresCanonicalIdentityFacts(t *testing.T) {
	raw, err := EncodeProviderInputSnapshot(ProviderInputSnapshot{
		Operation: "tool.generate_draft", Modality: "text", NormalizedText: `{"requirement":"count users"}`,
		MaxOutputTokens: 64, Prompt: "provider prompt", SystemPrompt: "strict json",
	})
	if err != nil {
		t.Fatalf("EncodeProviderInputSnapshot: %v", err)
	}
	got, err := DecodeProviderInputSnapshot(raw)
	if err != nil {
		t.Fatalf("DecodeProviderInputSnapshot: %v", err)
	}
	if got.Operation != "tool.generate_draft" || got.Modality != "text" || got.NormalizedText != `{"requirement":"count users"}` || got.MaxOutputTokens != 64 || got.Prompt != "provider prompt" || got.SystemPrompt != "strict json" {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestTextResultCandidateIsStrictAndPreservesAnswer(t *testing.T) {
	raw, err := MarshalResultCandidate("  paid answer  ")
	if err != nil {
		t.Fatalf("MarshalResultCandidate: %v", err)
	}
	answer, err := AnswerFromResultCandidate(raw)
	if err != nil {
		t.Fatalf("AnswerFromResultCandidate: %v", err)
	}
	if answer != "paid answer" {
		t.Fatalf("answer = %q", answer)
	}
	if _, err := AnswerFromResultCandidate(`{"version":"ai_text_result_v1","answer":"ok","extra":true}`); err == nil {
		t.Fatal("unknown candidate field should be rejected")
	}
}
