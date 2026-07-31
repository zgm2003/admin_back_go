package aichat

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

func TestNewRuntimeServiceRejectsMissingToolRuntime(t *testing.T) {
	service, err := NewRuntimeService(Dependencies{})
	if service != nil || !errors.Is(err, ErrToolRuntimeNotConfigured) {
		t.Fatalf("service=%#v err=%v", service, err)
	}
}

func TestNewRuntimeServiceRejectsMissingOfficialModelResolver(t *testing.T) {
	service, err := NewRuntimeService(Dependencies{ToolRuntime: &fakeToolRuntime{}})
	if service != nil || !errors.Is(err, ErrOfficialModelResolverNotConfigured) {
		t.Fatalf("service=%#v err=%v", service, err)
	}
}

func TestNewRuntimeServiceRejectsMissingDeliveryCommitter(t *testing.T) {
	service, err := NewRuntimeService(Dependencies{
		ToolRuntime:     &fakeToolRuntime{},
		PricingResolver: testCurrentPricingResolver(),
	})
	if service != nil || !errors.Is(err, ErrDeliveryCommitterNotConfigured) {
		t.Fatalf("service=%#v err=%v", service, err)
	}
}

func TestTextCompletionPricingSnapshotUsesInjectedResolver(t *testing.T) {
	resolverCalls := 0
	service := newTestChatService(Dependencies{PricingResolver: officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		resolverCalls++
		if modelID != "injected-chat-model" {
			t.Fatalf("resolver model = %q", modelID)
		}
		return testCurrentModelPrice(modelID, 2048), nil
	})})
	raw, effective, appErr := service.textCompletionPricingSnapshot(context.Background(), AgentEngineConfig{
		ModelID: "injected-chat-model", EngineType: "openai", ProviderModelStatus: enum.CommonYes,
		OfficialModelID: "injected-chat-model", OfficialCatalogVersion: "catalog-v3", MappingStatus: officialmodel.MappingStatusMapped,
		BillingMultiplierPPM: 1_250_000,
	})
	if appErr != nil || effective != 2048 || resolverCalls != 1 {
		t.Fatalf("snapshot result = %q, %d, %#v; calls=%d", raw, effective, appErr, resolverCalls)
	}
	snapshot, err := aigateway.ParsePricingSnapshot(raw)
	if err != nil || snapshot.SchemaVersion != aigateway.CurrentPricingSnapshotSchemaVersion || snapshot.PriceSource != "override" || snapshot.OverrideVersion != 3 || snapshot.MultiplierPPM != 1_250_000 {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
}

func testCurrentModelPrice(modelID string, maxOutput int64) officialmodel.ResolvedModel {
	rates := []pricing.Rate{
		{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
		{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
	}
	return officialmodel.ResolvedModel{
		Model: officialmodel.Model{
			CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelFamily: "gpt", ModelID: modelID,
			ContextWindowTokens: maxOutput * 4, MaxOutputTokens: maxOutput, OfficialPrice: pricing.PriceBook{ModelID: modelID, Rates: rates},
		},
		EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOverride,
		OverrideVersion: 3, PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
}

func testCurrentPricingResolver() officialmodel.Resolver {
	return officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		return testCurrentModelPrice(modelID, 2048), nil
	})
}

func newTestChatService(deps Dependencies) *Service {
	if deps.PricingResolver == nil {
		deps.PricingResolver = testCurrentPricingResolver()
	}
	if deps.DeliveryCommitter == nil {
		deps.DeliveryCommitter = &fakeDeliveryCommitter{}
	}
	return NewService(deps)
}

type fakeRepository struct {
	conversation       *Conversation
	agent              *AgentEngineConfig
	recoveryProvider   *AgentEngineConfig
	acceptedRun        *airun.Run
	agentID            uint64
	recoveryProviderID uint64
	conversationCalls  int
	agentCalls         int
	historyCalls       int
	history            []MessageHistory
	assistant          AssistantPublication
	createdRun         CreateRunRecord
	completedRun       CompleteRunRecord
	finishedRun        FinishRunRecord
	timeoutLimit       int
	staleBefore        time.Time
}

type fakeDurableTextService struct {
	replayInput  aitext.ReplayInput
	replayResult *aitext.Result
	replayFound  bool
	replayErr    *apperror.Error
	submitInput  aitext.AcceptInput
	submitResult *aitext.Result
	submitErr    *apperror.Error
	replayCalls  int
	submitCalls  int
}

func (f *fakeDurableTextService) ReplayAndWait(_ context.Context, input aitext.ReplayInput) (*aitext.Result, bool, *apperror.Error) {
	f.replayCalls++
	f.replayInput = input
	return f.replayResult, f.replayFound, f.replayErr
}

func (f *fakeDurableTextService) SubmitAndWait(_ context.Context, input aitext.AcceptInput) (*aitext.Result, *apperror.Error) {
	f.submitCalls++
	f.submitInput = input
	if f.submitResult == nil && f.submitErr == nil {
		return &aitext.Result{TaskID: 77, RunID: 99, RequestID: input.RequestID, Kind: aitext.KindText, Answer: "ok"}, nil
	}
	return f.submitResult, f.submitErr
}

type fakeRunRecorder struct {
	nextID      int64
	completeErr error
	started     airun.StartInput
	completed   airun.CompleteInput
	failed      airun.FailInput
	canceled    airun.CancelInput
	timeout     airun.TimeoutInput
}

func (f *fakeRunRecorder) Start(ctx context.Context, input airun.StartInput) (int64, error) {
	f.started = input
	if f.nextID == 0 {
		return 1, nil
	}
	return f.nextID, nil
}

func (f *fakeRunRecorder) Complete(ctx context.Context, input airun.CompleteInput) error {
	f.completed = input
	return f.completeErr
}

func (f *fakeRunRecorder) Fail(ctx context.Context, input airun.FailInput) error {
	f.failed = input
	return nil
}

func (f *fakeRunRecorder) Cancel(ctx context.Context, input airun.CancelInput) error {
	f.canceled = input
	return nil
}

func (f *fakeRunRecorder) Timeout(ctx context.Context, input airun.TimeoutInput) error {
	f.timeout = input
	return nil
}

func (f *fakeRepository) ConversationForReply(ctx context.Context, id int64, userID int64) (*Conversation, error) {
	f.conversationCalls++
	return f.conversation, nil
}
func (f *fakeRepository) AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error) {
	f.agentCalls++
	f.agentID = agentID
	return f.agent, nil
}
func (f *fakeRepository) LatestMessages(ctx context.Context, conversationID int64, limit int) ([]MessageHistory, error) {
	f.historyCalls++
	return f.history, nil
}
func (f *fakeRepository) ProviderForPreparedRecovery(_ context.Context, providerID uint64) (*AgentEngineConfig, error) {
	f.recoveryProviderID = providerID
	return f.recoveryProvider, nil
}
func (f *fakeRepository) AcceptedRunForReply(_ context.Context, _ int64, requestID string) (*airun.Run, error) {
	if f.acceptedRun != nil {
		return f.acceptedRun, nil
	}
	conversationID, userMessageID := int64(3), int64(9)
	providerID, modelID := int64(2), "gpt-5.4"
	if f.agent != nil {
		providerID, modelID = int64(f.agent.ProviderID), f.agent.ModelID
	}
	return &airun.Run{ID: 100, ConversationID: &conversationID, UserMessageID: &userMessageID, RequestID: requestID, UserID: 7, AgentID: 5, ProviderID: providerID, ModelID: modelID, Status: enum.AIRunStatusRunning, BillingStatus: "pending", BillingReason: "pending"}, nil
}
func (f *fakeRepository) PublishAssistant(_ context.Context, input AssistantPublication) (int64, bool, error) {
	f.assistant = input
	return 22, true, nil
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
func (f *fakeRepository) TimeoutRuns(ctx context.Context, limit int, staleBefore time.Time, message string) (int64, error) {
	f.timeoutLimit = limit
	f.staleBefore = staleBefore
	return 2, nil
}

type fakePublisher struct {
	pubs []infrarealtime.Publication
}

type fakeDeliveryCommitter struct {
	inputs    []DeliveryCommit
	nextSeq   uint32
	committed bool
	err       error
}

func (f *fakeDeliveryCommitter) CommitDelivery(_ context.Context, input DeliveryCommit) (uint32, bool, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return 0, false, f.err
	}
	f.nextSeq++
	committed := f.committed
	if !committed {
		committed = true
	}
	return f.nextSeq, committed, nil
}

type failingEventSink struct {
	err error
}

func (s failingEventSink) Emit(context.Context, infraai.Event) error {
	return s.err
}

type fakeAssistantPublisher struct {
	input AssistantPublication
}

type fakeProviderAttemptRecorder struct {
	events   []string
	prepared ProviderAttemptPrepareInput
	marked   ProviderAttemptMarkInput
	finished ProviderAttemptFinishInput
}

type fakePaidAttemptExecutor struct {
	result *PaidChatAttemptResult
	err    error
}

type recoveringPaidAttemptExecutor struct {
	hasPrepared bool
	probeRunID  int64
	probeCmdID  uint64
	input       PaidChatAttemptInput
}

func (f *recoveringPaidAttemptExecutor) HasPreparedPaidChatAttempt(_ context.Context, runID int64, commandID uint64) (bool, error) {
	f.probeRunID = runID
	f.probeCmdID = commandID
	return f.hasPrepared, nil
}

func (f *recoveringPaidAttemptExecutor) ExecutePaidChatAttempt(_ context.Context, input PaidChatAttemptInput) (*PaidChatAttemptResult, error) {
	f.input = input
	return &PaidChatAttemptResult{Finalized: true, AssistantMessageID: 22}, nil
}

type sequencePaidAttemptExecutor struct {
	results []*PaidChatAttemptResult
	inputs  []PaidChatAttemptInput
}

type finalizingPaidFailureExecutor struct {
	executeCalls      int
	preDispatchInputs []PaidChatAttemptInput
}

func (f *sequencePaidAttemptExecutor) ExecutePaidChatAttempt(_ context.Context, input PaidChatAttemptInput) (*PaidChatAttemptResult, error) {
	f.inputs = append(f.inputs, input)
	if len(f.results) == 0 {
		return nil, errors.New("unexpected paid attempt")
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, nil
}

func (f fakePaidAttemptExecutor) ExecutePaidChatAttempt(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error) {
	return f.result, f.err
}

func (f *finalizingPaidFailureExecutor) ExecutePaidChatAttempt(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error) {
	f.executeCalls++
	return nil, errors.New("unexpected paid attempt")
}

func (f *finalizingPaidFailureExecutor) FinalizePaidChatPreDispatchFailure(_ context.Context, input PaidChatAttemptInput) (*PaidChatAttemptResult, error) {
	f.preDispatchInputs = append(f.preDispatchInputs, input)
	return &PaidChatAttemptResult{Finalized: true}, nil
}

func (f *finalizingPaidFailureExecutor) FinalizePaidChatLocalFailure(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error) {
	return nil, errors.New("unexpected local failure finalization")
}

func (f *fakeProviderAttemptRecorder) PrepareProviderAttempt(_ context.Context, input ProviderAttemptPrepareInput) (*ProviderAttemptRef, error) {
	f.events = append(f.events, "prepared")
	f.prepared = input
	return &ProviderAttemptRef{ID: 91, IdempotencyKey: "provider-attempt-key"}, nil
}

func (f *fakeProviderAttemptRecorder) MarkProviderAttemptDispatched(_ context.Context, input ProviderAttemptMarkInput) error {
	f.events = append(f.events, "dispatched")
	f.marked = input
	return nil
}

func (f *fakeProviderAttemptRecorder) FinishProviderAttempt(_ context.Context, input ProviderAttemptFinishInput) error {
	f.events = append(f.events, "finished")
	f.finished = input
	return nil
}

func TestStreamChatWithAttemptReturnsPaidFinalization(t *testing.T) {
	service := &Service{paidAttemptExecutor: fakePaidAttemptExecutor{result: &PaidChatAttemptResult{
		ChatResult:         &infraai.ChatResult{Answer: "already finalized"},
		Finalized:          true,
		AssistantMessageID: 22,
	}}}

	result, err := service.streamChatWithAttempt(t.Context(), 100, ConversationReplyInput{CommandID: 41}, nil, infraai.ChatInput{}, nil)
	if err != nil {
		t.Fatalf("stream paid chat: %v", err)
	}
	if !result.Finalized || result.AssistantMessageID != 22 || result.ChatResult == nil || result.ChatResult.Answer != "already finalized" {
		t.Fatalf("result=%+v", result)
	}
}

type attemptCaptureEngine struct {
	input infraai.ChatInput
}

func (e *attemptCaptureEngine) TestConnection(context.Context, infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (e *attemptCaptureEngine) StreamChat(_ context.Context, input infraai.ChatInput, _ infraai.EventSink) (*infraai.ChatResult, error) {
	e.input = input
	return &infraai.ChatResult{Answer: "ok", UsageStatus: infraai.UsageStatusUnavailable, ProviderRequestID: "provider-request-1", ResponseSHA256: [32]byte{1}}, nil
}

func (f *fakeAssistantPublisher) PublishAssistant(_ context.Context, input AssistantPublication) (int64, bool, error) {
	f.input = input
	return 22, true, nil
}

func (f *fakePublisher) Publish(ctx context.Context, p infrarealtime.Publication) error {
	f.pubs = append(f.pubs, p)
	return nil
}

func TestDrainSinkPropagatesPublisherCancellationWhileDeliveryIsActive(t *testing.T) {
	want := infraai.ErrCanceled
	sink := newDrainSink(context.Background(), failingEventSink{err: want})

	if err := sink.Emit(context.Background(), infraai.Event{Type: "delta", DeltaText: "visible"}); !errors.Is(err, want) {
		t.Fatalf("Emit error=%v, want %v", err, want)
	}
}

type fakeEngineFactory struct {
	engine infraai.Engine
	input  EngineConfig
	err    error
}

type fakePreparedFileOpener struct{}

func (*fakePreparedFileOpener) Head(context.Context, infraai.PreparedFileOpenInput) (infraai.PreparedFileObjectMetadata, error) {
	return infraai.PreparedFileObjectMetadata{}, nil
}

func (*fakePreparedFileOpener) Open(context.Context, infraai.PreparedFileOpenInput) (io.ReadCloser, infraai.PreparedFileObjectMetadata, error) {
	return io.NopCloser(strings.NewReader("")), infraai.PreparedFileObjectMetadata{}, nil
}

func (f *fakeEngineFactory) NewEngine(ctx context.Context, input EngineConfig) (infraai.Engine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.engine, nil
}

type blankEngine struct {
	infraai.FakeEngine
}

func (blankEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	return &infraai.ChatResult{UsageStatus: infraai.UsageStatusUnavailable}, nil
}

type fakeEngine struct {
	result *infraai.ChatResult
	err    error
}

func (f *fakeEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (f *fakeEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type splitDeltaEngine struct{}

func (splitDeltaEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (splitDeltaEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	for _, delta := range []string{"你", "好"} {
		if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: delta, Payload: map[string]any{"delta": delta}}); err != nil {
			return nil, err
		}
	}
	return &infraai.ChatResult{Answer: "你好", PromptTokens: 4, CompletionTokens: 8, TotalTokens: 12, UsageStatus: infraai.UsageStatusReported}, nil
}

type captureEngine struct {
	input infraai.ChatInput
}

func (c *captureEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (c *captureEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	c.input = input
	return &infraai.ChatResult{Answer: "看到了图片", UsageStatus: infraai.UsageStatusUnavailable}, nil
}

type canceledEngine struct{}

func (canceledEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (canceledEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	return nil, context.Canceled
}

type stopThenDrainEngine struct {
	stopDelivery  context.CancelCauseFunc
	drainCanceled bool
}

func (e *stopThenDrainEngine) TestConnection(context.Context, infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (e *stopThenDrainEngine) StreamChat(ctx context.Context, _ infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: "停止前"}); err != nil {
		return nil, err
	}
	e.stopDelivery(infraai.ErrCanceled)
	if ctx.Err() != nil {
		e.drainCanceled = true
	}
	if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: "停止后"}); err != nil {
		return nil, err
	}
	rawUsage := []byte(`{"prompt_tokens":4,"completion_tokens":8}`)
	usage, err := infraai.NewUsageSnapshot(infraai.UsageStatusComplete, rawUsage, []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 4},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 8},
	})
	if err != nil {
		return nil, err
	}
	return &infraai.ChatResult{
		Answer: "停止后完整回答", PromptTokens: 4, CompletionTokens: 8, TotalTokens: 12,
		UsageStatus: infraai.UsageStatusReported, Usage: usage,
		ProviderRequestID: "provider-drained", DispatchState: infraai.DispatchStateDispatched,
		ResponseSHA256: sha256.Sum256([]byte("provider-response")),
	}, nil
}

func validAgentConfig(t *testing.T) (*AgentEngineConfig, secretbox.Box) {
	t.Helper()
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentEngineConfig{
		AgentID:                5,
		AgentName:              "客服",
		ProviderID:             2,
		ModelID:                "gpt-5.4",
		ModelDisplayName:       "GPT-5.4",
		ScenesJSON:             `["chat"]`,
		EngineType:             string(infraai.EngineTypeOpenAI),
		EngineBaseURL:          "https://api.openai.test/v1",
		EngineAPIKeyEnc:        cipher,
		AgentStatus:            enum.CommonYes,
		EngineStatus:           enum.CommonYes,
		ProviderModelStatus:    enum.CommonYes,
		OfficialModelID:        "gpt-5.4",
		OfficialCatalogVersion: "catalog-v3",
		MappingStatus:          officialmodel.MappingStatusMapped,
	}, box
}

func validGenerationTextAgentConfig(t *testing.T) (*AgentEngineConfig, secretbox.Box) {
	t.Helper()
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentEngineConfig{
		AgentID:                8,
		AgentName:              "Generation文本助手",
		ProviderID:             2,
		ModelID:                "gpt-4.1-mini",
		ModelDisplayName:       "GPT 4.1 Mini",
		SystemPrompt:           "用中文回答",
		ScenesJSON:             `["text_generate"]`,
		EngineType:             string(infraai.EngineTypeOpenAI),
		EngineBaseURL:          "https://api.openai.test/v1",
		EngineAPIKeyEnc:        cipher,
		AgentStatus:            enum.CommonYes,
		EngineStatus:           enum.CommonYes,
		ProviderModelStatus:    enum.CommonYes,
		OfficialModelID:        "gpt-4.1-mini",
		OfficialCatalogVersion: "catalog-v3",
		MappingStatus:          officialmodel.MappingStatusMapped,
		BillingMultiplierPPM:   1_000_000,
	}, box
}

func TestCompleteTextSubmitsDurableGatewayTaskWithoutDirectProviderCall(t *testing.T) {
	agent, _ := validGenerationTextAgentConfig(t)
	repo := &fakeRepository{agent: agent}
	textService := &fakeDurableTextService{submitResult: &aitext.Result{TaskID: 77, RunID: 99, RequestID: "request-1", Kind: aitext.KindText, Answer: "看到了图片"}}
	factory := &fakeEngineFactory{engine: &captureEngine{}}
	pub := &fakePublisher{}

	res, appErr := newTestChatService(Dependencies{
		Repository:         repo,
		AssistantPublisher: repo,
		Publisher:          pub,
		TextGeneration:     textService,
		EngineFactory:      factory,
		PricingResolver:    testCurrentPricingResolver(),
	}).CompleteText(context.Background(), TextCompletionInput{Platform: enum.PlatformAdmin, RequestID: "request-1", UserID: 7, AgentID: 8, ModelID: "client-model", Message: " hello text "})

	if appErr != nil {
		t.Fatalf("CompleteText returned error: %#v", appErr)
	}
	if res == nil || res.ID != "text-completion-77" || res.Object != "chat.completion" || res.Content != "看到了图片" {
		t.Fatalf("unexpected response: %#v", res)
	}
	if repo.agentID != 8 {
		t.Fatalf("expected runtime agent id 8, got %d", repo.agentID)
	}
	if textService.replayCalls != 1 || textService.submitCalls != 1 {
		t.Fatalf("durable text calls: replay=%d submit=%d", textService.replayCalls, textService.submitCalls)
	}
	if textService.submitInput.RequestID != "request-1" || textService.submitInput.Kind != aitext.KindText || textService.submitInput.AgentID != 8 || textService.submitInput.ModelID != "gpt-4.1-mini" || textService.submitInput.RequestFingerprint == ([32]byte{}) || textService.submitInput.PricingSnapshotJSON == "" || textService.submitInput.InputSnapshot == "" {
		t.Fatalf("durable text acceptance input=%#v", textService.submitInput)
	}
	if repo.createdRun.ConversationID != 0 || repo.createdRun.UserMessageID != 0 || repo.assistant.Content != "" || len(pub.pubs) != 0 {
		t.Fatalf("stateless completion must not persist or publish: repo=%#v pubs=%#v", repo, pub.pubs)
	}
	if factory.input != (EngineConfig{}) {
		t.Fatalf("HTTP text path called provider factory directly: %#v", factory.input)
	}
}

func TestCompleteTextReplaysBeforeCurrentAgentLookup(t *testing.T) {
	textService := &fakeDurableTextService{
		replayFound:  true,
		replayResult: &aitext.Result{TaskID: 77, RunID: 99, RequestID: "request-replay", Kind: aitext.KindText, Answer: "persisted answer"},
	}
	repo := &fakeRepository{}

	res, appErr := newTestChatService(Dependencies{Repository: repo, TextGeneration: textService}).CompleteText(
		context.Background(),
		TextCompletionInput{Platform: enum.PlatformAdmin, RequestID: "request-replay", UserID: 7, AgentID: 8, Message: "same input"},
	)

	if appErr != nil || res == nil || res.Content != "persisted answer" {
		t.Fatalf("durable replay result=%#v error=%v", res, appErr)
	}
	if repo.agentID != 0 || textService.submitCalls != 0 {
		t.Fatalf("replay consulted mutable agent or submitted new work: agent=%d submits=%d", repo.agentID, textService.submitCalls)
	}
}

func TestCompleteTextRejectsMissingOrUnregisteredPlatformBeforeRepository(t *testing.T) {
	for _, platform := range []string{"", "partner_portal"} {
		repo := &fakeRepository{}
		_, appErr := newTestChatService(Dependencies{Repository: repo}).CompleteText(context.Background(), TextCompletionInput{Platform: platform, RequestID: "request-1", UserID: 7, AgentID: 8, Message: "hello"})
		if appErr == nil || appErr.MessageID != "aitext.platform.invalid" {
			t.Fatalf("expected platform %q to be rejected, got %#v", platform, appErr)
		}
		if repo.agentID != 0 {
			t.Fatalf("invalid platform %q reached repository", platform)
		}
	}
}

func TestCompleteTextRejectsNonGenerationTextScene(t *testing.T) {
	agent, _ := validAgentConfig(t)
	_, appErr := newTestChatService(Dependencies{
		Repository:     &fakeRepository{agent: agent},
		TextGeneration: &fakeDurableTextService{},
	}).CompleteText(context.Background(), TextCompletionInput{Platform: enum.PlatformAdmin, RequestID: "request-1", UserID: 7, AgentID: 5, Message: "hi"})
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.MessageID != "aitext.agent_unavailable" {
		t.Fatalf("expected generation text scene rejection, got %#v", appErr)
	}
}

func TestCompleteTextRejectsEmptySettledAnswer(t *testing.T) {
	agent, _ := validGenerationTextAgentConfig(t)
	_, appErr := newTestChatService(Dependencies{
		Repository:      &fakeRepository{agent: agent},
		PricingResolver: testCurrentPricingResolver(),
		TextGeneration: &fakeDurableTextService{submitResult: &aitext.Result{
			TaskID: 77, RunID: 99, RequestID: "request-1", Kind: aitext.KindText,
		}},
	}).CompleteText(context.Background(), TextCompletionInput{Platform: enum.PlatformAdmin, RequestID: "request-1", UserID: 7, AgentID: 8, Message: "hi"})
	if appErr == nil || appErr.MessageID != "aitext.empty_result" {
		t.Fatalf("expected empty result error, got %#v", appErr)
	}
}

func TestExecuteDurableConversationReplyPublishesOnlyEphemeralEventsAndPersistsAssistant(t *testing.T) {
	agent, box := validAgentConfig(t)
	agent.APIProtocol = "responses"
	fileOpener := &fakePreparedFileOpener{}
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history: []MessageHistory{
			{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"},
		},
	}
	pub := &fakePublisher{}
	factory := &fakeEngineFactory{engine: infraai.NewFakeEngine("ok")}
	recorder := &fakeRunRecorder{nextID: 100}
	attempts := &fakeProviderAttemptRecorder{}
	res, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, AttemptRecorder: attempts, Publisher: pub, RunRecorder: recorder, EngineFactory: factory, FileOpener: fileOpener, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 2, ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "ok" || repo.assistant.ConversationID != 3 {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if factory.input.APIKey != "provider-key" || factory.input.EngineType != infraai.EngineTypeOpenAI || factory.input.APIProtocol != "responses" || factory.input.FileOpener != fileOpener {
		t.Fatalf("unexpected engine config: %#v", factory.input)
	}
	if recorder.started != (airun.StartInput{}) {
		t.Fatalf("paid reply reopened its accepted run: %#v", recorder.started)
	}
	if recorder.completed != (airun.CompleteInput{}) {
		t.Fatalf("paid reply bypassed the billing finalizer: %#v", recorder.completed)
	}
	if attempts.prepared.RunID != 100 || attempts.finished.RunID != 100 {
		t.Fatalf("attempt did not reuse accepted run: prepared=%+v finished=%+v", attempts.prepared, attempts.finished)
	}
	if len(pub.pubs) != 2 || pub.pubs[0].Envelope.Type != EventAIResponseStart || pub.pubs[1].Envelope.Type != EventAIResponseDelta {
		t.Fatalf("unexpected publications: %#v", pub.pubs)
	}
	for _, pub := range pub.pubs {
		if pub.UserID != 7 || pub.Platform != enum.PlatformAdmin {
			t.Fatalf("publication not scoped to current admin user: %#v", pub)
		}
	}
}

func TestExecuteDurableConversationReplyDoesNotReportFailureAfterTerminalCommit(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	recorder := &fakeRunRecorder{nextID: 100, completeErr: errors.New("run recorder unavailable after commit")}
	res, err := newTestChatService(Dependencies{
		Repository: repo, AssistantPublisher: repo, AttemptRecorder: &fakeProviderAttemptRecorder{},
		Publisher: &fakePublisher{}, RunRecorder: recorder,
		EngineFactory: &fakeEngineFactory{engine: infraai.NewFakeEngine("ok")}, Secretbox: box,
	}).ExecuteConversationReply(t.Context(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 2,
		ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid",
	})
	if err != nil || res == nil || res.AssistantMessageID != 22 {
		t.Fatalf("committed assistant result was reported as failed: result=%#v err=%v", res, err)
	}
}

func TestRecoveredNativeFileAttemptUsesPersistedManifestOnly(t *testing.T) {
	_, box := validAgentConfig(t)
	cipher, err := box.Encrypt("recovery-provider-key")
	if err != nil {
		t.Fatal(err)
	}
	conversationID, userMessageID := int64(3), int64(9)
	repo := &fakeRepository{
		recoveryProvider: &AgentEngineConfig{
			AgentID: 5, ProviderID: 2, EngineType: "mutated-transport", APIProtocol: "chat_completions",
			EngineBaseURL: "https://current-provider.test/v1", EngineAPIKeyEnc: cipher,
		},
		acceptedRun: &airun.Run{
			ID: 100, UserID: 7, AgentID: 5, ProviderID: 2, ModelID: "gpt-5.4", RequestID: "rid",
			ConversationID: &conversationID, UserMessageID: &userMessageID,
			PricingSnapshotJSON: `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`,
			Status:              enum.AIRunStatusRunning, BillingStatus: "held", BillingReason: "held",
		},
	}
	paid := &recoveringPaidAttemptExecutor{hasPrepared: true}
	factory := &fakeEngineFactory{engine: infraai.NewFakeEngine("must not stream mutable input")}
	fileOpener := &fakePreparedFileOpener{}
	service := newTestChatService(Dependencies{
		Repository: repo, PaidAttemptExecutor: paid, Publisher: &fakePublisher{},
		EngineFactory: factory, FileOpener: fileOpener, Secretbox: box,
	})

	result, err := service.ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 2, ConversationID: 3, UserID: 7,
		AgentID: 5, UserMessageID: 9, RequestID: "rid", CommandAttempt: 2, CommandMaxAttempts: 3,
	})

	if err != nil || result == nil || !result.Finalized || result.AssistantMessageID != 22 {
		t.Fatalf("recovery result=%+v err=%v", result, err)
	}
	if paid.probeRunID != 100 || paid.probeCmdID != 41 || paid.input.RunID != 100 || paid.input.CommandID != 41 {
		t.Fatalf("recovery probe/input=%+v input=%+v", paid, paid.input)
	}
	if repo.conversationCalls != 0 || repo.agentCalls != 0 || repo.historyCalls != 0 || repo.recoveryProviderID != 2 {
		t.Fatalf("mutable context was consulted: conversation=%d agent=%d history=%d recovery_provider=%d", repo.conversationCalls, repo.agentCalls, repo.historyCalls, repo.recoveryProviderID)
	}
	if factory.input.EngineType != infraai.EngineTypeOpenAI || factory.input.BaseURL != "https://current-provider.test/v1" ||
		factory.input.APIKey != "recovery-provider-key" || factory.input.APIProtocol != "chat_completions" || factory.input.FileOpener != fileOpener {
		t.Fatalf("recovery engine config=%+v", factory.input)
	}
	if paid.input.ChatInput.Content != "" || len(paid.input.ChatInput.Inputs) != 0 || !reflect.DeepEqual(paid.input.RequestIdentity, requestidentity.Input{}) {
		t.Fatalf("mutable request identity leaked into recovery: %+v", paid.input)
	}
}

func TestExecuteConversationReplyPublishesAssistantThroughFencedBoundary(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	assistantPublisher := &fakeAssistantPublisher{}
	res, err := newTestChatService(Dependencies{
		Repository:         repo,
		AssistantPublisher: assistantPublisher,
		AttemptRecorder:    &fakeProviderAttemptRecorder{},
		Publisher:          &fakePublisher{},
		RunRecorder:        &fakeRunRecorder{nextID: 100},
		EngineFactory:      &fakeEngineFactory{engine: infraai.NewFakeEngine("ok")},
		Secretbox:          box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 7,
		ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "rid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.AssistantMessageID != 22 || assistantPublisher.input.CommandID != 41 || assistantPublisher.input.Owner != "worker-a" || assistantPublisher.input.Token != 7 || assistantPublisher.input.Content != "ok" {
		t.Fatalf("result=%+v publication=%+v", res, assistantPublisher.input)
	}
}

func TestExecuteConversationReplyPersistsProviderAttemptAroundNetworkCall(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	attempts := &fakeProviderAttemptRecorder{}
	engine := &attemptCaptureEngine{}
	_, err := newTestChatService(Dependencies{
		Repository:         repo,
		AssistantPublisher: repo,
		AttemptRecorder:    attempts,
		Publisher:          &fakePublisher{},
		RunRecorder:        &fakeRunRecorder{nextID: 100},
		EngineFactory:      &fakeEngineFactory{engine: engine},
		Secretbox:          box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 7,
		ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "rid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(attempts.events, ",") != "prepared,dispatched,finished" {
		t.Fatalf("attempt events=%v", attempts.events)
	}
	if attempts.prepared.CommandID != 41 || attempts.prepared.Owner != "worker-a" || attempts.prepared.Token != 7 || attempts.marked.AttemptID != 91 || attempts.finished.State != ProviderAttemptSucceeded || attempts.finished.ProviderRequestID != "provider-request-1" || len(attempts.finished.ResponseSHA256) != 64 {
		t.Fatalf("prepared=%+v marked=%+v finished=%+v", attempts.prepared, attempts.marked, attempts.finished)
	}
	if engine.input.AttemptID != 91 || engine.input.IdempotencyKey != "provider-attempt-key" {
		t.Fatalf("engine input=%+v", engine.input)
	}
}

func TestExecuteConversationReplyDoesNotCompleteIncompleteReportedUsage(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	attempts := &fakeProviderAttemptRecorder{}
	_, err := newTestChatService(Dependencies{
		Repository: repo, AssistantPublisher: repo, AttemptRecorder: attempts,
		Publisher: &fakePublisher{}, RunRecorder: &fakeRunRecorder{nextID: 100},
		EngineFactory: &fakeEngineFactory{engine: &fakeEngine{result: &infraai.ChatResult{Answer: "ok", UsageStatus: infraai.UsageStatusReported}}}, Secretbox: box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 7,
		ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "rid-reported",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts.finished.UsageStatus != infraai.UsageStatusUnavailable {
		t.Fatalf("incomplete reported usage was upgraded: %+v", attempts.finished)
	}
}

func TestExecuteConversationReplyRecordsAmbiguousProviderOutcome(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	attempts := &fakeProviderAttemptRecorder{}
	providerErr := infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "provider-request-2", errors.New("stream disconnected"))
	_, err := newTestChatService(Dependencies{
		Repository:         repo,
		AssistantPublisher: repo,
		AttemptRecorder:    attempts,
		Publisher:          &fakePublisher{},
		RunRecorder:        &fakeRunRecorder{nextID: 100},
		EngineFactory:      &fakeEngineFactory{engine: &fakeEngine{err: providerErr}},
		Secretbox:          box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 42, LeaseOwner: "worker-a", LeaseToken: 8,
		ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "rid-2",
	})
	if !errors.Is(err, providerErr) {
		t.Fatalf("err=%v", err)
	}
	if attempts.finished.State != ProviderAttemptOutcomeUnknown || attempts.finished.ErrorCode != "ai.provider_outcome_unknown" || attempts.finished.ProviderRequestID != "provider-request-2" {
		t.Fatalf("finished=%+v", attempts.finished)
	}
}

func TestStreamChatWithAttemptMapsProviderErrorsToStrictTerminalEvidence(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name              string
		providerErr       error
		localCancel       bool
		wantState         ProviderAttemptState
		wantDispatchState string
		wantErrorCode     string
		wantProviderID    string
	}{
		{
			name:              "rejected request was dispatched",
			providerErr:       infraai.NewProviderError(infraai.ProviderOutcomeRejected, "provider-rejected", errors.New("rejected")),
			wantState:         ProviderAttemptFailed,
			wantDispatchState: infraai.DispatchStateDispatched,
			wantErrorCode:     "ai.provider_failed",
			wantProviderID:    "provider-rejected",
		},
		{
			name:              "request was not dispatched",
			providerErr:       infraai.NewProviderError(infraai.ProviderOutcomeNotDispatched, "", errors.New("dial failed")),
			wantState:         ProviderAttemptFailed,
			wantDispatchState: infraai.DispatchStateNotDispatched,
			wantErrorCode:     "ai.provider_failed",
		},
		{
			name:              "provider outcome is unknown",
			providerErr:       infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "provider-unknown", errors.New("stream disconnected")),
			wantState:         ProviderAttemptOutcomeUnknown,
			wantDispatchState: infraai.DispatchStateUnknown,
			wantErrorCode:     "ai.provider_outcome_unknown",
			wantProviderID:    "provider-unknown",
		},
		{
			name:              "unclassified error is unknown",
			providerErr:       errors.New("legacy provider error"),
			wantState:         ProviderAttemptOutcomeUnknown,
			wantDispatchState: infraai.DispatchStateUnknown,
			wantErrorCode:     "ai.provider_outcome_unknown",
		},
		{
			name:              "local cancellation before dispatch",
			providerErr:       infraai.NewProviderError(infraai.ProviderOutcomeNotDispatched, "", context.Canceled),
			localCancel:       true,
			wantState:         ProviderAttemptCanceled,
			wantDispatchState: infraai.DispatchStateNotDispatched,
			wantErrorCode:     "ai.provider_canceled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.localCancel {
				canceledCtx, cancel := context.WithCancelCause(ctx)
				cancel(infraai.ErrCanceled)
				ctx = canceledCtx
			}
			attempts := &fakeProviderAttemptRecorder{}
			service := &Service{attemptRecorder: attempts, now: func() time.Time { return now }}

			_, err := service.streamChatWithAttempt(ctx, 100, ConversationReplyInput{
				CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 7,
			}, &fakeEngine{err: tc.providerErr}, infraai.ChatInput{}, nil)

			if !errors.Is(err, tc.providerErr) {
				t.Fatalf("err=%v, want provider error %v", err, tc.providerErr)
			}
			if strings.Join(attempts.events, ",") != "prepared,dispatched,finished" {
				t.Fatalf("attempt events=%v", attempts.events)
			}
			finished := attempts.finished
			if finished.State != tc.wantState || finished.DispatchState != tc.wantDispatchState || finished.ErrorCode != tc.wantErrorCode || finished.ProviderRequestID != tc.wantProviderID {
				t.Fatalf("finished=%+v", finished)
			}
			if finished.UsageStatus != infraai.UsageStatusUnavailable || finished.UsageJSON != `{"status":"unavailable"}` {
				t.Fatalf("usage evidence=%+v", finished)
			}
		})
	}
}

func TestExecuteConversationReplyDerivesAgentFromOwnedConversation(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	_, err := newTestChatService(Dependencies{
		Repository:         repo,
		AssistantPublisher: repo,
		Publisher:          &fakePublisher{},
		RunRecorder:        &fakeRunRecorder{nextID: 100},
		EngineFactory:      &fakeEngineFactory{engine: infraai.NewFakeEngine("ok")},
		Secretbox:          box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if repo.agentID != 5 {
		t.Fatalf("runtime agent id=%d, want conversation agent 5", repo.agentID)
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
	delivery := &fakeDeliveryCommitter{}
	recorder := &fakeRunRecorder{nextID: 100}
	res, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, DeliveryCommitter: delivery, Publisher: pub, RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: splitDeltaEngine{}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "你好" {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if recorder.completed.TotalTokens != 12 || recorder.completed.PromptTokens != 4 || recorder.completed.CompletionTokens != 8 {
		t.Fatalf("run token usage was not persisted: %#v", recorder.completed)
	}
	if len(delivery.inputs) != 1 || delivery.inputs[0].Delta != "你好" {
		t.Fatalf("delivery commits=%+v", delivery.inputs)
	}
	var deltas []DeltaPayload
	for _, pub := range pub.pubs {
		if pub.Envelope.Type != EventAIResponseDelta {
			continue
		}
		var payload DeltaPayload
		if err := json.Unmarshal(pub.Envelope.Data, &payload); err != nil {
			t.Fatalf("unexpected delta payload: %v", err)
		}
		deltas = append(deltas, payload)
	}
	if len(deltas) != 1 || deltas[0].DeliverySeq != 1 || deltas[0].Delta != "你好" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestExecuteConversationReplyStopsDeliveryButDrainsUsageAndCandidate(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"}},
	}
	deliveryCtx, stopDelivery := context.WithCancelCause(context.Background())
	engine := &stopThenDrainEngine{stopDelivery: stopDelivery}
	attempts := &fakeProviderAttemptRecorder{}
	publisher := &fakePublisher{}
	delivery := &fakeDeliveryCommitter{}
	recorder := &fakeRunRecorder{nextID: 100}

	result, err := newTestChatService(Dependencies{
		Repository: repo, AssistantPublisher: repo, DeliveryCommitter: delivery, AttemptRecorder: attempts, Publisher: publisher,
		RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 2, DeliveryContext: deliveryCtx,
		ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid-drain",
	})

	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if result == nil || !result.DeliveryStopped || result.AssistantMessageID != 0 {
		t.Fatalf("result=%+v", result)
	}
	if engine.drainCanceled {
		t.Fatal("delivery stop canceled provider drain context")
	}
	var deltas []string
	for _, publication := range publisher.pubs {
		if publication.Envelope.Type != EventAIResponseDelta {
			continue
		}
		var payload DeltaPayload
		if err := json.Unmarshal(publication.Envelope.Data, &payload); err != nil {
			t.Fatal(err)
		}
		deltas = append(deltas, payload.Delta)
	}
	if len(deltas) != 0 || len(delivery.inputs) != 0 {
		t.Fatalf("delivered deltas=%v", deltas)
	}
	if repo.assistant.Content != "" || recorder.completed.RunID != 0 || recorder.canceled.RunID != 0 {
		t.Fatalf("canceled delivery published terminal state: assistant=%+v complete=%+v canceled=%+v", repo.assistant, recorder.completed, recorder.canceled)
	}
	finished := attempts.finished
	if finished.State != ProviderAttemptCanceled || finished.DispatchState != infraai.DispatchStateDispatched || finished.UsageStatus != infraai.UsageStatusComplete || finished.ErrorCode != "ai.user_stopped" {
		t.Fatalf("finished attempt=%+v", finished)
	}
	var usage infraai.UsageSnapshot
	if err := json.Unmarshal([]byte(finished.UsageJSON), &usage); err != nil || !usage.Complete() {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	if finished.ResultCandidateJSON == nil {
		t.Fatal("result candidate was not persisted")
	}
	var candidate struct {
		Version string `json:"version"`
		Answer  string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(*finished.ResultCandidateJSON), &candidate); err != nil || candidate.Version != "ai_chat_result_v2" || candidate.Answer != "停止后完整回答" {
		t.Fatalf("candidate=%+v raw=%v err=%v", candidate, finished.ResultCandidateJSON, err)
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
	recorder := &fakeRunRecorder{nextID: 100}

	res, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: &fakePublisher{}, RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})

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
	if !strings.Contains(recorder.started.InputSnapshot, "attachments") || strings.TrimSpace(recorder.started.InputSnapshot) == "" {
		t.Fatalf("image-only run snapshot must use source message metadata, got %q", recorder.started.InputSnapshot)
	}
}

func TestAssistantMessageZeroValueMetaJSONIsNil(t *testing.T) {
	message := Message{ConversationID: 3, Role: enum.AIMessageRoleAssistant, ContentType: "text", Content: "ok", IsDel: enum.CommonNo}
	if message.MetaJSON != nil {
		t.Fatalf("assistant message without metadata must keep meta_json nil, got %#v", message.MetaJSON)
	}
}

func TestExecuteDurableConversationReplyDoesNotPublishUncommittedFailure(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}}}
	pub := &fakePublisher{}
	recorder := &fakeRunRecorder{nextID: 100}
	_, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, AttemptRecorder: &fakeProviderAttemptRecorder{}, Publisher: pub, RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: &infraai.FakeEngine{Err: errors.New("engine down")}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 2, ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected engine error")
	}
	for _, publication := range pub.pubs {
		if publication.Envelope.Type == EventAIResponseFailed || publication.Envelope.Type == EventAIResponseCompleted {
			t.Fatalf("durable terminal event was published outside command transaction: %#v", pub.pubs)
		}
	}
	if recorder.failed != (airun.FailInput{}) {
		t.Fatalf("paid run was terminalized outside the finalizer: %#v", recorder.failed)
	}
}

func TestExecuteConversationReplyMarksRunCanceledForCanceledContext(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}}}
	pub := &fakePublisher{}
	recorder := &fakeRunRecorder{nextID: 100}
	_, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: pub, RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: canceledEngine{}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if recorder.canceled.RunID != 100 {
		t.Fatalf("run cancellation not persisted: %#v", recorder.canceled)
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
	res, err := newTestChatService(Dependencies{
		Repository:         repo,
		AssistantPublisher: repo,
		Publisher:          pub,
		RunRecorder:        &fakeRunRecorder{nextID: 100},
		EngineFactory:      &fakeEngineFactory{engine: &blankEngine{}},
		Secretbox:          box,
	}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "AI没有返回内容" {
		t.Fatalf("unexpected fallback result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if len(pub.pubs) < 2 || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseDelta {
		t.Fatalf("expected fallback delta, got %#v", pub.pubs)
	}
	for _, publication := range pub.pubs {
		if publication.Envelope.Type == EventAIResponseCompleted || publication.Envelope.Type == EventAIResponseFailed {
			t.Fatalf("terminal events must only be emitted by the durable command transaction: %#v", pub.pubs)
		}
	}
}

func TestTimeoutRunsMarksOldRunsFailed(t *testing.T) {
	repo := &fakeRepository{}
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	res, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, RunStaleTimeout: 20 * time.Minute, Now: func() time.Time { return now }}).TimeoutRuns(context.Background(), RunTimeoutInput{Limit: 5})
	if err != nil {
		t.Fatalf("TimeoutRuns returned error: %v", err)
	}
	if repo.timeoutLimit != 5 || res.Failed != 2 {
		t.Fatalf("unexpected timeout result: repo=%#v res=%#v", repo, res)
	}
	if want := now.Add(-20 * time.Minute); !repo.staleBefore.Equal(want) {
		t.Fatalf("expected staleBefore %s, got %s", want, repo.staleBefore)
	}
}

func TestTimeoutRunsAllowsInputStaleTimeoutOverride(t *testing.T) {
	repo := &fakeRepository{}
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	_, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, RunStaleTimeout: 20 * time.Minute, Now: func() time.Time { return now }}).
		TimeoutRuns(context.Background(), RunTimeoutInput{Limit: 5, StaleTimeout: 7 * time.Minute})
	if err != nil {
		t.Fatalf("TimeoutRuns returned error: %v", err)
	}
	if want := now.Add(-7 * time.Minute); !repo.staleBefore.Equal(want) {
		t.Fatalf("expected staleBefore %s, got %s", want, repo.staleBefore)
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
	history, ok := inputs["history"].([]map[string]any)
	if !ok || len(history) != 1 || history[0]["content"] != "older" {
		t.Fatalf("max_history not applied: %#v", inputs["history"])
	}
}

func TestNativeFileContextIncludesSelectedHistoryAndCurrentMessage(t *testing.T) {
	historyMeta := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/history.pdf","etag":"\"h1\"","mime_type":"application/pdf","name":"history.pdf","size":26214400,"url":"https://example.test/history.pdf"}]}`
	currentAtLimit := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/current.pdf","etag":"\"c1\"","mime_type":"application/pdf","name":"current.pdf","size":26214400,"url":"https://example.test/current.pdf"}]}`
	currentOverLimit := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/current.pdf","etag":"\"c1\"","mime_type":"application/pdf","name":"current.pdf","size":26214401,"url":"https://example.test/current.pdf"}]}`

	messages := []MessageHistory{
		{ID: 1, Role: enum.AIMessageRoleUser, Content: "history", MetaJSON: &historyMeta},
		{ID: 2, Role: enum.AIMessageRoleAssistant, Content: "answer"},
		{ID: 3, Role: enum.AIMessageRoleUser, Content: "current", MetaJSON: &currentAtLimit},
	}
	if appErr := requireNativeFileContextWithinLimit(messages); appErr != nil {
		t.Fatalf("exactly 50 MiB was rejected: %#v", appErr)
	}
	inputs := chatInputs(AgentEngineConfig{ModelID: "gpt-test"}, messages, 3)
	selected, ok := inputs["history"].([]map[string]any)
	if !ok || len(selected) != 2 {
		t.Fatalf("selected history=%#v", inputs["history"])
	}
	attachments, ok := selected[0]["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("history attachments=%#v", selected[0]["attachments"])
	}

	messages[2].MetaJSON = &currentOverLimit
	appErr := requireNativeFileContextWithinLimit(messages)
	if appErr == nil || appErr.Code != "ai.attachment.context_total_too_large" || appErr.Category != apperror.CategoryValidation ||
		appErr.HTTPStatus != 400 || appErr.Retry != apperror.Permanent || appErr.Message != "当前对话文件上下文超过 50 MB，请新建对话或减少历史范围" {
		t.Fatalf("over-limit error=%#v", appErr)
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
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, f.executeErr.Error(), f.executeErr)
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
	_, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: &fakePublisher{}, RunRecorder: &fakeRunRecorder{nextID: 100}, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box, KnowledgeRuntime: knowledge}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
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
	res, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: &fakePublisher{}, RunRecorder: &fakeRunRecorder{nextID: 100}, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box, KnowledgeRuntime: knowledge}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("knowledge failure must not block chat: %v", err)
	}
	if res.AssistantMessageID != 22 || engine.input.Content != "hi" {
		t.Fatalf("chat should continue with original message: res=%#v input=%q", res, engine.input.Content)
	}
}

type toolCallEngine struct {
	calls []infraai.ChatInput
}

func (e *toolCallEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (e *toolCallEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	e.calls = append(e.calls, input)
	if len(input.ToolOutputs) == 0 {
		return &infraai.ChatResult{
			ToolCalls: []infraai.ToolCall{{ID: "call-1", Name: "admin_user_count", Arguments: "{}"}},
			Continuation: &infraai.ChatContinuation{
				Protocol: infraai.APIProtocolResponses,
				Items:    []byte(`[{"id":"rs_1","type":"reasoning","encrypted_content":"opaque"},{"id":"fc_1","type":"function_call","call_id":"call-1","name":"admin_user_count","arguments":"{}"}]`),
			},
			PromptTokens: 7, CompletionTokens: 1, TotalTokens: 8, UsageStatus: infraai.UsageStatusReported,
		}, nil
	}
	return &infraai.ChatResult{Answer: "当前用户量1015", PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5, UsageStatus: infraai.UsageStatusReported}, nil
}

type missingFirstToolUsageEngine struct {
	calls []infraai.ChatInput
}

func (e *missingFirstToolUsageEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}

func (e *missingFirstToolUsageEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	e.calls = append(e.calls, input)
	if len(input.ToolOutputs) == 0 {
		return &infraai.ChatResult{ToolCalls: []infraai.ToolCall{{ID: "call-1", Name: "admin_user_count", Arguments: "{}"}}, PromptTokens: 7, CompletionTokens: 1, TotalTokens: 8}, nil
	}
	return &infraai.ChatResult{Answer: "当前用户量1015", PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5, UsageStatus: infraai.UsageStatusReported}, nil
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
	recorder := &fakeRunRecorder{nextID: 100}
	res, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: pub, RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: engine}, Secretbox: box, ToolRuntime: runtime}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
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
	if engine.calls[1].Continuation == nil || engine.calls[1].Continuation.Protocol != infraai.APIProtocolResponses ||
		!strings.Contains(string(engine.calls[1].Continuation.Items), `"encrypted_content":"opaque"`) {
		t.Fatalf("Responses continuation not preserved for second model request: %#v", engine.calls[1].Continuation)
	}
	if len(runtime.executeInput) != 1 || runtime.executeInput[0].Tool.Code != "admin_user_count" {
		t.Fatalf("tool runtime not executed: %#v", runtime.executeInput)
	}
	if recorder.completed.TotalTokens != 13 || recorder.completed.PromptTokens != 9 || recorder.completed.CompletionTokens != 4 {
		t.Fatalf("tool round token usage must include both model requests: %#v", recorder.completed)
	}
}

func TestPaidConversationReplyCarriesToolsAcrossTwoPreparedAttempts(t *testing.T) {
	agent, box := validAgentConfig(t)
	identity := requestidentity.Input{
		UserID: 7, Operation: "chat.reply", Modality: "chat", AgentID: 5, ModelID: "gpt-5.4",
		NormalizedText: "查用户量", ConversationID: 3,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: 10},
	}
	fingerprint, err := requestidentity.Fingerprint(identity)
	if err != nil {
		t.Fatal(err)
	}
	conversationID, userMessageID := int64(3), int64(9)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "查用户量"}},
		acceptedRun: &airun.Run{
			ID: 100, UserID: 7, AgentID: 5, ProviderID: 2, ModelID: "gpt-5.4", RequestID: "rid",
			ConversationID: &conversationID, UserMessageID: &userMessageID,
			RequestFingerprint: fingerprint[:], RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
			PricingSnapshotJSON: `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`,
			Status:              enum.AIRunStatusRunning, BillingStatus: "pending", BillingReason: "pending",
		},
	}
	firstUsage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":{"prompt_tokens":2}}`), []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 2}})
	if err != nil {
		t.Fatal(err)
	}
	secondUsage, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, []byte(`{"usage":{"completion_tokens":3}}`), []infraai.UsageItem{{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 3}})
	if err != nil {
		t.Fatal(err)
	}
	recovered := &infraai.ChatResult{ToolCalls: []infraai.ToolCall{{ID: "call-1", Name: "admin_user_count", Arguments: "{}"}}, Usage: firstUsage, UsageStatus: infraai.UsageStatusReported, PromptTokens: 2, TotalTokens: 2}
	continuation := &infraai.ChatResult{Answer: "当前用户量1015", UsageStatus: infraai.UsageStatusReported, Usage: secondUsage, CompletionTokens: 3, TotalTokens: 3}
	paid := &sequencePaidAttemptExecutor{results: []*PaidChatAttemptResult{{ChatResult: recovered}, {ChatResult: continuation}}}
	runtime := &fakeToolRuntime{runtimeTools: []RuntimeTool{{ID: 1, Name: "查询当前用户量", Code: "admin_user_count", Description: "查询后台当前用户数量，只返回数量。", ParametersJSON: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, RiskLevel: "low", TimeoutMS: 3000}}}
	service := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, PaidAttemptExecutor: paid, Publisher: &fakePublisher{}, RunRecorder: &fakeRunRecorder{nextID: 100}, EngineFactory: &fakeEngineFactory{engine: infraai.NewFakeEngine("unused")}, Secretbox: box, ToolRuntime: runtime})

	result, err := service.ExecuteConversationReply(context.Background(), ConversationReplyInput{CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 1, ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil || result == nil || result.AssistantMessageID != 22 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(runtime.executeInput) != 1 || len(paid.inputs) != 2 || len(paid.inputs[0].ChatInput.Tools) != 1 || len(paid.inputs[0].ChatInput.ToolOutputs) != 0 || len(paid.inputs[1].ChatInput.Tools) != 1 || len(paid.inputs[1].ChatInput.ToolCalls) != 1 || len(paid.inputs[1].ChatInput.ToolOutputs) != 1 {
		t.Fatalf("tool executions=%+v paid inputs=%+v", runtime.executeInput, paid.inputs)
	}
	if !firstUsage.Complete() || !secondUsage.Complete() || recovered.Usage.RawProviderJSON == nil || continuation.Usage.RawProviderJSON == nil {
		t.Fatalf("both provider attempts must carry complete reported usage: first=%+v second=%+v", recovered.Usage, continuation.Usage)
	}
}

func TestPaidReplyRequestIdentityUsesAcceptedCOSObjectKeyForAttachments(t *testing.T) {
	const pricingSnapshot = `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`
	const objectKey = "ai_chat_images/2026/07/29/example.jpg"
	meta := `{"attachments":[{"type":"image","object_key":"` + objectKey + `","url":"https://cos.example.test/` + objectKey + `"}]}`
	acceptedIdentity := requestidentity.Input{
		UserID: 7, Operation: "chat.reply", Modality: "chat", AgentID: 5, ModelID: "gpt-5.4",
		NormalizedText: "看图", ConversationID: 3,
		Attachments: []requestidentity.AttachmentIdentity{{StorageProvider: "cos", StorageKey: objectKey}},
		Options:     requestidentity.GenerationOptions{MaxOutputTokens: 10},
	}
	fingerprint, err := requestidentity.Fingerprint(acceptedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	run := &airun.Run{
		AgentID: 5, ModelID: "gpt-5.4", RequestFingerprint: fingerprint[:],
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), PricingSnapshotJSON: pricingSnapshot,
	}

	identity, err := paidReplyRequestIdentity(run, ConversationReplyInput{ConversationID: 3, UserID: 7}, MessageHistory{ID: 9, Content: "看图", MetaJSON: &meta})

	if err != nil {
		t.Fatalf("paidReplyRequestIdentity returned error: %v", err)
	}
	if len(identity.Attachments) != 1 || identity.Attachments[0].StorageProvider != "cos" || identity.Attachments[0].StorageKey != objectKey {
		t.Fatalf("unexpected attachment identity: %#v", identity.Attachments)
	}
}

func TestPaidReplyRequestIdentityUsesCompleteTrustedAttachmentFacts(t *testing.T) {
	const pricingSnapshot = `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`
	facts := requestidentity.AttachmentFacts{
		Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", ETag: `"v1"`,
		Size: 4096, MIMEType: "application/pdf", Name: "report.pdf",
	}
	digest, err := requestidentity.AttachmentFactsSHA256(facts)
	if err != nil {
		t.Fatal(err)
	}
	metaBytes, err := json.Marshal(map[string]any{"attachments": []map[string]any{{
		"type": facts.Type, "object_key": facts.ObjectKey, "etag": facts.ETag, "size": facts.Size,
		"mime_type": facts.MIMEType, "name": facts.Name, "url": "https://cos.example.test/report.pdf",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	meta := string(metaBytes)
	acceptedIdentity := requestidentity.Input{
		UserID: 7, Operation: "chat.reply", Modality: "chat", AgentID: 5, ModelID: "gpt-5.4",
		NormalizedText: "总结文件", ConversationID: 3, PreserveAttachmentOrder: true,
		Attachments: []requestidentity.AttachmentIdentity{{StorageProvider: "cos", StorageKey: facts.ObjectKey, SHA256: digest}},
		Options:     requestidentity.GenerationOptions{MaxOutputTokens: 10},
	}
	fingerprint, err := requestidentity.BuildFingerprint(acceptedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	run := &airun.Run{
		AgentID: 5, ModelID: "gpt-5.4", RequestFingerprint: fingerprint[:],
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), PricingSnapshotJSON: pricingSnapshot,
	}

	identity, err := paidReplyRequestIdentity(run, ConversationReplyInput{ConversationID: 3, UserID: 7}, MessageHistory{ID: 9, Content: "总结文件", MetaJSON: &meta})

	if err != nil {
		t.Fatalf("paidReplyRequestIdentity returned error: %v", err)
	}
	if !identity.PreserveAttachmentOrder || len(identity.Attachments) != 1 || identity.Attachments[0].SHA256 != digest {
		t.Fatalf("unexpected attachment identity: %#v", identity)
	}
}

func TestPaidReplyRequestIdentityFallsBackAsAWholeForHistoricalAttachmentFacts(t *testing.T) {
	const pricingSnapshot = `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`
	const canonicalKey = "ai_chat_attachments/2026/07/new.pdf"
	const legacyKey = "ai_chat_images/2026/07/old.jpg"
	meta := `{"attachments":[{"type":"file","object_key":"` + canonicalKey + `","etag":"\"v1\"","size":4096,"mime_type":"application/pdf","name":"new.pdf"},{"type":"image","object_key":"` + legacyKey + `","url":"https://cos.example.test/old.jpg"}]}`
	acceptedIdentity := requestidentity.Input{
		UserID: 7, Operation: "chat.reply", Modality: "chat", AgentID: 5, ModelID: "gpt-5.4",
		NormalizedText: "比较附件", ConversationID: 3,
		Attachments: []requestidentity.AttachmentIdentity{
			{StorageProvider: "cos", StorageKey: legacyKey},
			{StorageProvider: "cos", StorageKey: canonicalKey},
		},
		Options: requestidentity.GenerationOptions{MaxOutputTokens: 10},
	}
	fingerprint, err := requestidentity.BuildFingerprint(acceptedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	run := &airun.Run{
		AgentID: 5, ModelID: "gpt-5.4", RequestFingerprint: fingerprint[:],
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), PricingSnapshotJSON: pricingSnapshot,
	}

	identity, err := paidReplyRequestIdentity(run, ConversationReplyInput{ConversationID: 3, UserID: 7}, MessageHistory{ID: 9, Content: "比较附件", MetaJSON: &meta})

	if err != nil {
		t.Fatalf("historical attachment identity returned error: %v", err)
	}
	if identity.PreserveAttachmentOrder || len(identity.Attachments) != 2 {
		t.Fatalf("historical attachment identity=%#v", identity)
	}
	for _, attachment := range identity.Attachments {
		if attachment.SHA256 != "" {
			t.Fatalf("partially canonical historical identity retained SHA: %#v", identity.Attachments)
		}
	}
}

func TestPaidReplyRequestIdentityRestoresRevisionContextFromRunSnapshot(t *testing.T) {
	const pricingSnapshot = `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`
	const objectKey = "ai_chat_images/2026/07/29/revision.jpg"
	meta := `{"attachments":[{"type":"image","object_key":"` + objectKey + `","url":"https://cos.example.test/` + objectKey + `"}]}`
	acceptedIdentity := requestidentity.Input{
		UserID: 7, Operation: "chat.revision", Modality: "chat", AgentID: 5, ModelID: "gpt-5.4",
		NormalizedText: "改后的问题", ConversationID: 3, SourceMessageID: 41,
		Attachments: []requestidentity.AttachmentIdentity{{StorageProvider: "cos", StorageKey: objectKey}},
		Options:     requestidentity.GenerationOptions{MaxOutputTokens: 10},
	}
	fingerprint, err := requestidentity.Fingerprint(acceptedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	run := &airun.Run{
		AgentID: 5, ModelID: "gpt-5.4", RequestFingerprint: fingerprint[:],
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), PricingSnapshotJSON: pricingSnapshot,
		InputSnapshot: `{"content":"改后的问题","request_identity":{"operation":"chat.revision","source_message_id":41}}`,
	}

	identity, err := paidReplyRequestIdentity(run, ConversationReplyInput{ConversationID: 3, UserID: 7}, MessageHistory{ID: 71, Content: "改后的问题", MetaJSON: &meta})

	if err != nil {
		t.Fatalf("paidReplyRequestIdentity returned error: %v", err)
	}
	if identity.Operation != "chat.revision" || identity.SourceMessageID != 41 {
		t.Fatalf("unexpected revision identity: %#v", identity)
	}
}

func TestPaidConversationReplyFinalizesAttachmentIdentityMismatchBeforeDispatch(t *testing.T) {
	agent, box := validAgentConfig(t)
	conversationID, userMessageID := int64(3), int64(9)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "看图"}},
		acceptedRun: &airun.Run{
			ID: 100, UserID: 7, AgentID: 5, ProviderID: 2, ModelID: "gpt-5.4", RequestID: "rid",
			ConversationID: &conversationID, UserMessageID: &userMessageID,
			RequestFingerprint: make([]byte, sha256.Size), RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
			PricingSnapshotJSON: `{"version":"test-v1","billable":true,"catalog_vendor":"test","transport_engine":"openai","requested_model_id":"gpt-5.4","canonical_model_id":"gpt-5.4","catalog_max_output_tokens":100,"effective_max_output_tokens":10,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`,
			Status:              enum.AIRunStatusRunning, BillingStatus: "pending", BillingReason: "pending",
		},
	}
	paid := &finalizingPaidFailureExecutor{}
	service := newTestChatService(Dependencies{
		Repository: repo, Publisher: &fakePublisher{}, PaidAttemptExecutor: paid,
		EngineFactory: &fakeEngineFactory{engine: infraai.NewFakeEngine("unused")}, Secretbox: box,
	})

	result, err := service.ExecuteConversationReply(context.Background(), ConversationReplyInput{
		CommandID: 41, LeaseOwner: "worker-a", LeaseToken: 1, ConversationID: 3, UserID: 7,
		AgentID: 5, UserMessageID: 9, RequestID: "rid", CommandAttempt: 1, CommandMaxAttempts: 3,
	})

	if err != nil || result == nil || !result.Finalized {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(paid.preDispatchInputs) != 1 || paid.preDispatchInputs[0].RunID != 100 || paid.executeCalls != 0 {
		t.Fatalf("unexpected paid failure finalization: %+v execute_calls=%d", paid.preDispatchInputs, paid.executeCalls)
	}
}

func TestExecuteConversationReplyRejectsMissingToolRoundUsageStatus(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history:      []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "查用户量"}},
	}
	runtime := &fakeToolRuntime{runtimeTools: []RuntimeTool{{ID: 1, Name: "查询当前用户量", Code: "admin_user_count", Description: "查询后台当前用户数量，只返回数量。", ParametersJSON: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, RiskLevel: "low", TimeoutMS: 3000}}}
	recorder := &fakeRunRecorder{nextID: 100}

	_, err := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: &fakePublisher{}, RunRecorder: recorder, EngineFactory: &fakeEngineFactory{engine: &missingFirstToolUsageEngine{}}, Secretbox: box, ToolRuntime: runtime}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})

	if err == nil || !strings.Contains(err.Error(), "用量状态缺失") {
		t.Fatalf("expected missing usage status error, got %v", err)
	}
	if recorder.completed.RunID != 0 {
		t.Fatalf("run must not complete when provider usage status is missing: %#v", recorder.completed)
	}
	if recorder.failed.RunID != 100 {
		t.Fatalf("run failure not persisted: %#v", recorder.failed)
	}
}

func TestExecuteConversationReplyRejectsSecondToolRound(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "查用户量"}}}
	runtime := &fakeToolRuntime{runtimeTools: []RuntimeTool{{ID: 1, Name: "查询当前用户量", Code: "admin_user_count", Description: "查询后台当前用户数量，只返回数量。", ParametersJSON: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, RiskLevel: "low", TimeoutMS: 3000}}}
	service := newTestChatService(Dependencies{Repository: repo, AssistantPublisher: repo, Publisher: &fakePublisher{}, RunRecorder: &fakeRunRecorder{nextID: 100}, EngineFactory: &fakeEngineFactory{engine: &doubleToolRoundEngine{}}, Secretbox: box, ToolRuntime: runtime})
	_, err := service.ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected second round error")
	}
}

type doubleToolRoundEngine struct{}

func (doubleToolRoundEngine) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return &infraai.TestConnectionResult{OK: true}, nil
}
func (doubleToolRoundEngine) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	return &infraai.ChatResult{ToolCalls: []infraai.ToolCall{{ID: "call-1", Name: "admin_user_count", Arguments: "{}"}}}, nil
}
