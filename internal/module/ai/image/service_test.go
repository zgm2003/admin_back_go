package aiimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/modelpricing"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type fakeImageRepository struct {
	agent           *AgentRuntime
	task            *ImageTask
	files           []ImageFile
	createdTask     ImageTask
	createdFiles    TaskFileSet
	appendedFiles   []ImageFile
	claimTask       bool
	leaseClaim      bool
	renewLost       bool
	nextTaskID      uint64
	deletePlatform  string
	failedID        uint64
	failedMsg       string
	successID       uint64
	successJSON     *string
	successRaw      *string
	listAgentsScene string
	listCalls       int
	loadAgentCalls  int
	workerPlatform  string
	uploadConfig    *UploadConfig
	accepted        map[string]ImageTask
	acceptInputs    []AcceptTaskInput
	acceptedGraphs  int
}

func (f *fakeImageRepository) ListImageAgents(_ context.Context, scene string) ([]AgentOption, error) {
	f.listAgentsScene = scene
	return nil, nil
}
func (f *fakeImageRepository) ListTasks(context.Context, uint64, ListQuery) ([]ImageTask, int64, error) {
	f.listCalls++
	return nil, 0, nil
}
func (f *fakeImageRepository) GetTask(_ context.Context, userID uint64, taskID uint64, platform string) (*ImageTask, error) {
	if f.task != nil && f.task.UserID == userID && f.task.ID == taskID && (platform == "" || f.task.Platform == platform) {
		row := *f.task
		return &row, nil
	}
	if f.createdTask.ID == taskID && f.createdTask.UserID == userID && (platform == "" || f.createdTask.Platform == platform) {
		row := f.createdTask
		return &row, nil
	}
	return nil, nil
}
func (f *fakeImageRepository) GetTaskForWorker(ctx context.Context, platform string, userID uint64, taskID uint64) (*ImageTask, error) {
	f.workerPlatform = platform
	return f.GetTask(ctx, userID, taskID, platform)
}
func (f *fakeImageRepository) LoadTaskFiles(context.Context, uint64) ([]ImageFile, error) {
	return append([]ImageFile(nil), f.files...), nil
}
func (f *fakeImageRepository) CreateTaskWithFiles(_ context.Context, task ImageTask, files TaskFileSet) (uint64, error) {
	if f.nextTaskID == 0 {
		f.nextTaskID = 900
	}
	task.ID = f.nextTaskID
	f.createdTask = task
	f.createdFiles = TaskFileSet{Inputs: append([]ImageFile(nil), files.Inputs...)}
	if files.Mask != nil {
		mask := *files.Mask
		f.createdFiles.Mask = &mask
	}
	return f.nextTaskID, nil
}
func (f *fakeImageRepository) FindAcceptedTaskByRequestID(_ context.Context, userID uint64, requestID string) (*AcceptedTaskReplay, error) {
	key := fmt.Sprintf("%d:%s", userID, strings.TrimSpace(requestID))
	task, ok := f.accepted[key]
	if !ok {
		return nil, nil
	}
	for index := len(f.acceptInputs) - 1; index >= 0; index-- {
		accepted := f.acceptInputs[index]
		if accepted.Task.UserID == userID && accepted.Task.RequestID == strings.TrimSpace(requestID) {
			return &AcceptedTaskReplay{Task: task, InputSnapshot: accepted.InputSnapshot, PricingSnapshotJSON: accepted.PricingSnapshotJSON}, nil
		}
	}
	return nil, requestidentity.ErrRequestIdentityNotReplayable
}
func (f *fakeImageRepository) DeleteTask(_ context.Context, _ uint64, _ uint64, platform string) error {
	f.deletePlatform = platform
	return nil
}
func (f *fakeImageRepository) LoadAgentRuntime(_ context.Context, agentID uint64) (*AgentRuntime, error) {
	f.loadAgentCalls++
	if f.agent == nil || f.agent.AgentID != agentID {
		return nil, nil
	}
	row := *f.agent
	return &row, nil
}
func (f *fakeImageRepository) ClaimTask(_ context.Context, platform string, _ uint64, _ uint64, _ time.Time) (bool, error) {
	f.workerPlatform = platform
	return f.claimTask, nil
}
func (f *fakeImageRepository) ClaimTaskLease(_ context.Context, platform string, _ uint64, taskID uint64, owner string, now time.Time, ttl time.Duration) (*TaskLease, error) {
	f.workerPlatform = platform
	if !f.leaseClaim || f.task == nil {
		return nil, nil
	}
	task := *f.task
	return &TaskLease{Task: task, Owner: owner, Token: 1, ExpiresAt: now.Add(ttl)}, nil
}
func (f *fakeImageRepository) RenewTaskLease(context.Context, uint64, string, uint64, time.Time, time.Time) (bool, error) {
	return !f.renewLost, nil
}
func (f *fakeImageRepository) AppendTaskFiles(_ context.Context, files []ImageFile) error {
	f.appendedFiles = append(f.appendedFiles, files...)
	return nil
}
func (f *fakeImageRepository) FinishTaskSuccess(_ context.Context, platform string, _ uint64, taskID uint64, actual *string, raw *string, _ int, _ time.Time) error {
	f.workerPlatform = platform
	f.successID = taskID
	f.successJSON = actual
	f.successRaw = raw
	return nil
}
func (f *fakeImageRepository) FinishTaskFailed(_ context.Context, platform string, _ uint64, taskID uint64, message string, _ int, _ time.Time) error {
	f.workerPlatform = platform
	f.failedID = taskID
	f.failedMsg = message
	return nil
}
func (f *fakeImageRepository) LoadUploadConfig(context.Context) (*UploadConfig, error) {
	return f.uploadConfig, nil
}

type fakeImageEnqueuer struct{ tasks []taskqueue.Task }

func (f *fakeImageEnqueuer) Enqueue(_ context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	f.tasks = append(f.tasks, task)
	return taskqueue.EnqueueResult{ID: "task-1", Queue: task.Queue, Type: task.Type}, nil
}

type fakeImageEngineFactory struct {
	config ImageEngineConfig
	engine infraai.ImageEngine
}

func (f *fakeImageEngineFactory) NewImageEngine(config ImageEngineConfig) infraai.ImageEngine {
	f.config = config
	return f.engine
}

type fakeImageEngine struct {
	input  infraai.ImageInput
	result *infraai.ImageResult
	err    error
}

type fakeObjectWriter struct {
	input  storagecos.PutInput
	inputs []storagecos.PutInput
}

func (f *fakeObjectWriter) Put(_ context.Context, input storagecos.PutInput) error {
	f.input = input
	f.inputs = append(f.inputs, input)
	return nil
}

func (f *fakeImageEngine) GenerateImages(_ context.Context, input infraai.ImageInput) (*infraai.ImageResult, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type fakeImageRunRecorder struct {
	started   airun.StartInput
	completed airun.CompleteInput
	failed    airun.FailInput
}

func (f *fakeImageRunRecorder) Start(context.Context, airun.StartInput) (int64, error) {
	panic("use StartWithInput")
}
func (f *fakeImageRunRecorder) StartWithInput(input airun.StartInput) (int64, error) { return 0, nil }
func (f *fakeImageRunRecorder) Complete(_ context.Context, input airun.CompleteInput) error {
	f.completed = input
	return nil
}
func (f *fakeImageRunRecorder) Fail(_ context.Context, input airun.FailInput) error {
	f.failed = input
	return nil
}
func (f *fakeImageRunRecorder) Cancel(context.Context, airun.CancelInput) error   { return nil }
func (f *fakeImageRunRecorder) Timeout(context.Context, airun.TimeoutInput) error { return nil }

type recordingRunRecorder struct{ fakeImageRunRecorder }

func (f *recordingRunRecorder) Start(_ context.Context, input airun.StartInput) (int64, error) {
	f.started = input
	return 600, nil
}

func TestPageInitListsGenerationImageGenerateAgentsOnly(t *testing.T) {
	repo := &fakeImageRepository{}
	service := NewService(Dependencies{Repository: repo})

	result, appErr := service.PageInit(context.Background())

	if appErr != nil {
		t.Fatalf("expected page init to pass, got %#v", appErr)
	}
	if repo.listAgentsScene != capability.SceneImageGenerate {
		t.Fatalf("image page-init must list image_generate agents only, got %q", repo.listAgentsScene)
	}
	gotSizes := make(map[string]bool)
	for _, option := range result.Dict.SizeArr {
		gotSizes[option.Value] = true
	}
	for _, want := range []string{"1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792"} {
		if !gotSizes[want] {
			t.Fatalf("image page-init must expose supported Generation image size %q, got %#v", want, result.Dict.SizeArr)
		}
	}
}

type fakeImageTaskExecutor struct {
	taskID      uint64
	ctxCanceled bool
	status      string
	err         error
	block       bool
}

func (f *fakeImageTaskExecutor) ExecuteImageTask(ctx context.Context, taskID uint64) (string, error) {
	f.taskID = taskID
	f.ctxCanceled = ctx.Err() != nil
	if f.block {
		<-ctx.Done()
		f.ctxCanceled = true
		return "", ctx.Err()
	}
	return f.status, f.err
}
func (f *fakeImageRepository) AcceptTaskWithFiles(_ context.Context, input AcceptTaskInput) (*ImageTask, error) {
	f.acceptInputs = append(f.acceptInputs, input)
	if f.accepted == nil {
		f.accepted = make(map[string]ImageTask)
	}
	key := fmt.Sprintf("%d:%s", input.Task.UserID, input.Task.RequestID)
	if existing, ok := f.accepted[key]; ok {
		if !bytes.Equal(existing.RequestFingerprint, input.Task.RequestFingerprint) {
			return nil, requestidentity.ErrRequestIdentityConflict
		}
		row := existing
		return &row, nil
	}
	if f.nextTaskID == 0 {
		f.nextTaskID = 900
	}
	input.Task.ID = f.nextTaskID
	input.Task.RunID = int64(f.nextTaskID + 1000)
	f.accepted[key] = input.Task
	f.createdTask = input.Task
	f.createdFiles = input.Files
	f.acceptedGraphs++
	row := input.Task
	return &row, nil
}

func TestCreateRequiresRequestIDBeforeRepositoryOrQueue(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77}
	enqueuer := &fakeImageEnqueuer{}
	service := NewService(Dependencies{Repository: repo, Enqueuer: enqueuer, Secretbox: box, Now: fixedImageNow()})

	_, appErr := service.Create(context.Background(), CreateInput{
		UserID: 9, AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "draw a cat",
	})

	if appErr == nil || appErr.HTTPStatus != 400 {
		t.Fatalf("blank request_id error=%#v, want HTTP 400", appErr)
	}
	if repo.createdTask.ID != 0 {
		t.Fatalf("blank request_id persisted task %#v", repo.createdTask)
	}
	if len(enqueuer.tasks) != 0 {
		t.Fatalf("blank request_id enqueued tasks %#v", enqueuer.tasks)
	}
}

func TestCreateReplaysCanonicalRequestAndRejectsFingerprintConflictBeforeQueue(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77}
	enqueuer := &fakeImageEnqueuer{}
	service := NewService(Dependencies{Repository: repo, Enqueuer: enqueuer, Secretbox: box, Now: fixedImageNow()})
	input := CreateInput{
		UserID: 9, RequestID: " image-request-1 ", AgentID: 1, Platform: enum.PlatformAdmin,
		Prompt: "  draw a cat  ", Size: "1024x1024", Quality: "high", N: 1,
	}

	first, appErr := service.Create(context.Background(), input)
	if appErr != nil {
		t.Fatalf("first create error=%#v", appErr)
	}
	repo.agent.AgentStatus = enum.CommonNo
	replay := input
	replay.RequestID = "image-request-1"
	replay.Prompt = "draw a cat"
	second, appErr := service.Create(context.Background(), replay)
	if appErr != nil {
		t.Fatalf("replay error=%#v", appErr)
	}
	if first.Task.ID != second.Task.ID || repo.acceptedGraphs != 1 {
		t.Fatalf("replay created a different graph: first=%d second=%d graphs=%d", first.Task.ID, second.Task.ID, repo.acceptedGraphs)
	}
	if repo.loadAgentCalls != 1 {
		t.Fatalf("replay consulted mutable agent runtime %d times", repo.loadAgentCalls)
	}
	queuedBeforeConflict := len(enqueuer.tasks)
	conflict := replay
	conflict.Quality = "low"
	_, appErr = service.Create(context.Background(), conflict)
	if appErr == nil || appErr.HTTPStatus != 409 || appErr.Code != requestidentity.ErrorCodeFingerprintConflict {
		t.Fatalf("fingerprint conflict error=%#v, want stable HTTP 409", appErr)
	}
	if repo.acceptedGraphs != 1 || len(enqueuer.tasks) != queuedBeforeConflict {
		t.Fatalf("conflict crossed durable/queue boundary: graphs=%d queue=%d before=%d", repo.acceptedGraphs, len(enqueuer.tasks), queuedBeforeConflict)
	}
	if len(repo.acceptInputs) != 1 || len(repo.acceptInputs[0].Task.RequestFingerprint) != 32 {
		t.Fatalf("service did not pass canonical fingerprint: %#v", repo.acceptInputs)
	}
}

func TestCreateWithUploadedFilesReplaysBeforeUploadOrMutableAgentLookup(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77, uploadConfig: testUploadConfig(t, box)}
	writer := &fakeObjectWriter{}
	service := NewService(Dependencies{Repository: repo, Enqueuer: &fakeImageEnqueuer{}, Secretbox: box, ObjectWriter: writer, Now: fixedImageNow(), Random: fixedImageRandom})
	input := CreateWithUploadedFilesInput{
		CreateInput: CreateInput{UserID: 9, RequestID: "image-upload-replay", AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "draw"},
		Files:       []UploadedFileInput{{FileName: "reference.png", MimeType: "image/png", Body: []byte("same-image-bytes")}},
	}

	first, appErr := service.CreateWithUploadedFiles(context.Background(), input)
	if appErr != nil {
		t.Fatalf("first create error=%#v", appErr)
	}
	repo.agent.AgentStatus = enum.CommonNo
	second, appErr := service.CreateWithUploadedFiles(context.Background(), input)
	if appErr != nil {
		t.Fatalf("replay error=%#v", appErr)
	}
	if first.Task.ID != second.Task.ID || len(writer.inputs) != 1 || repo.loadAgentCalls != 1 {
		t.Fatalf("replay task=%d/%d uploads=%d agent_lookups=%d", first.Task.ID, second.Task.ID, len(writer.inputs), repo.loadAgentCalls)
	}
}

func TestCreateWithUploadedFilesRejectsChangedBytesBeforeUpload(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77, uploadConfig: testUploadConfig(t, box)}
	writer := &fakeObjectWriter{}
	service := NewService(Dependencies{Repository: repo, Enqueuer: &fakeImageEnqueuer{}, Secretbox: box, ObjectWriter: writer, Now: fixedImageNow(), Random: fixedImageRandom})
	input := CreateWithUploadedFilesInput{
		CreateInput: CreateInput{UserID: 9, RequestID: "image-upload-conflict", AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "draw"},
		Files:       []UploadedFileInput{{FileName: "reference.png", MimeType: "image/png", Body: []byte("original-image-bytes")}},
	}
	if _, appErr := service.CreateWithUploadedFiles(context.Background(), input); appErr != nil {
		t.Fatalf("first create error=%#v", appErr)
	}

	input.Files[0].Body = []byte("changed-image-bytes")
	_, appErr := service.CreateWithUploadedFiles(context.Background(), input)
	if appErr == nil || appErr.HTTPStatus != 409 || len(writer.inputs) != 1 {
		t.Fatalf("conflict error=%#v uploads=%d, want HTTP 409 before second upload", appErr, len(writer.inputs))
	}
}

func TestCreateEnqueuesGenerationTaskWithTaskOwnedFiles(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77}
	enqueuer := &fakeImageEnqueuer{}
	service := NewService(Dependencies{Repository: repo, Enqueuer: enqueuer, Secretbox: box, Now: fixedImageNow()})

	result, appErr := service.Create(context.Background(), CreateInput{
		UserID: 9, RequestID: "image-create-1", AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "  draw a cat  ",
		InputFiles: []ImageFileInput{{StorageProvider: StorageProviderCOS, StorageKey: "inputs/ref.png", StorageURL: "https://cos.test/ref.png", MimeType: "image/png", SizeBytes: 10, SHA256: strings.Repeat("a", 64)}},
		MaskFile:   &MaskFileInput{ImageFileInput: ImageFileInput{StorageProvider: StorageProviderCOS, StorageKey: "inputs/mask.png", StorageURL: "https://cos.test/mask.png", MimeType: "image/png", SizeBytes: 12, SHA256: strings.Repeat("b", 64)}, RelatedSortOrder: 1},
	})

	if appErr != nil {
		t.Fatalf("expected create to pass, got %#v", appErr)
	}
	if result.Task.ID != 77 || result.Task.Status != StatusPending {
		t.Fatalf("unexpected create response: %#v", result.Task)
	}
	if repo.createdTask.UserID != 9 || repo.createdTask.Platform != enum.PlatformAdmin || repo.createdTask.AgentNameSnapshot != "图片助手" || repo.createdTask.Prompt != "draw a cat" {
		t.Fatalf("generation task snapshot mismatch: %#v", repo.createdTask)
	}
	if len(repo.createdFiles.Inputs) != 1 || repo.createdFiles.Inputs[0].Role != FileRoleInput {
		t.Fatalf("input file was not task-owned: %#v", repo.createdFiles.Inputs)
	}
	if repo.createdFiles.Mask == nil || repo.createdFiles.Mask.File.Role != FileRoleMask || repo.createdFiles.Mask.RelatedSortOrder != 1 {
		t.Fatalf("mask file was not task-owned: %#v", repo.createdFiles.Mask)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type != TypeGenerateV1 {
		t.Fatalf("expected one generation image queue task, got %#v", enqueuer.tasks)
	}
}

func TestCreateAcceptsGenerationWideAndPortraitImageSizes(t *testing.T) {
	for _, size := range []string{"1792x1024", "1024x1792"} {
		t.Run(size, func(t *testing.T) {
			box := testImageSecretBox()
			repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77}
			service := NewService(Dependencies{Repository: repo, Enqueuer: &fakeImageEnqueuer{}, Secretbox: box, Now: fixedImageNow()})

			_, appErr := service.Create(context.Background(), CreateInput{
				UserID: 9, RequestID: "image-size-" + size, AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "draw", Size: size,
			})

			if appErr != nil {
				t.Fatalf("expected create to accept size %s, got %#v", size, appErr)
			}
			if repo.createdTask.Size != size {
				t.Fatalf("expected task size %s, got %s", size, repo.createdTask.Size)
			}
		})
	}
}

func TestGPTImage2OutputTokenUpperBoundBindsFinalSizeQualityAndCount(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		quality string
		count   int
		want    int64
	}{
		{name: "square low", size: "1024x1024", quality: "low", count: 1, want: 196},
		{name: "square medium", size: "1024x1024", quality: "medium", count: 1, want: 1756},
		{name: "square high", size: "1024x1024", quality: "high", count: 1, want: 7024},
		{name: "landscape high", size: "1536x1024", quality: "high", count: 1, want: 5488},
		{name: "portrait high", size: "1024x1536", quality: "high", count: 1, want: 5488},
		{name: "count is linear", size: "1024x1024", quality: "high", count: 15, want: 105360},
		{name: "auto quality reserves high", size: "1024x1024", quality: "auto", count: 1, want: 7024},
		{name: "auto size reserves global maximum", size: "auto", quality: "high", count: 1, want: 23719},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := gptImage2OutputTokenUpperBound(test.size, test.quality, test.count)
			if err != nil || got != test.want {
				t.Fatalf("bound=%d err=%v, want %d", got, err, test.want)
			}
		})
	}
}

func TestImagePricingSnapshotUsesRequestBoundAndRejectsSmallerAgentCap(t *testing.T) {
	resolverCalls := 0
	service := NewService(Dependencies{PricingResolver: modelpricing.ResolverFunc(func(_ context.Context, modelID string) (pricing.ModelPrice, error) {
		resolverCalls++
		return pricing.ModelPrice{
			Version: "catalog-v3", CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: modelID,
			MaxOutputTokens: 200000, PriceSource: "official", SourceURL: "https://openai.com/pricing", RetrievedAt: "2026-07-27",
			Rates: []pricing.Rate{{Category: pricing.MediaUnits, Unit: "image", PriceUnits: 1, UnitScale: 1}},
		}, nil
	})})
	agent := AgentRuntime{
		ModelID: RequiredModelID, EngineType: string(infraai.EngineTypeOpenAI),
		BillingMultiplierPPM: 1_000_000, MaxOutputTokens: 105360,
	}
	request := CreateInput{Size: "1024x1024", Quality: "high", N: 15}
	raw, effective, appErr := service.imagePricingSnapshot(context.Background(), agent, request)
	if appErr != nil || effective != 105360 || resolverCalls != 1 {
		t.Fatalf("effective=%d error=%#v", effective, appErr)
	}
	snapshot, err := aigateway.ParsePricingSnapshot(raw)
	if err != nil || int64(snapshot.EffectiveMaxOutputTokens) != effective || snapshot.SchemaVersion != aigateway.CurrentPricingSnapshotSchemaVersion {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}

	agent.MaxOutputTokens = uint(effective - 1)
	if _, _, appErr = service.imagePricingSnapshot(context.Background(), agent, request); appErr == nil || appErr.HTTPStatus != 409 {
		t.Fatalf("smaller agent cap error=%#v, want HTTP 409", appErr)
	}
}

func TestCreateRejectsUnregisteredImagePlatform(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box)}
	service := NewService(Dependencies{Repository: repo, Enqueuer: &fakeImageEnqueuer{}, Secretbox: box})

	_, appErr := service.Create(context.Background(), CreateInput{UserID: 9, RequestID: "image-invalid-platform", AgentID: 1, Platform: "partner_portal", Prompt: "draw"})

	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.MessageID != "aiimage.platform.invalid" {
		t.Fatalf("expected unregistered image platform to be rejected, got %#v", appErr)
	}
	if repo.createdTask.ID != 0 {
		t.Fatalf("unregistered image task must not be created: %#v", repo.createdTask)
	}
}

func TestDetailRejectsUnregisteredImagePlatform(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task}
	service := NewService(Dependencies{Repository: repo, Secretbox: box})

	_, appErr := service.Detail(context.Background(), task.UserID, task.ID, "partner_portal")

	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.MessageID != "aiimage.platform.invalid" {
		t.Fatalf("expected unregistered image platform to be rejected, got %#v", appErr)
	}
}

func TestListRejectsMissingOrUnregisteredImagePlatformBeforeRepository(t *testing.T) {
	for _, platform := range []string{"", "partner_portal"} {
		repo := &fakeImageRepository{}
		_, appErr := NewService(Dependencies{Repository: repo}).List(context.Background(), 9, ListQuery{Platform: platform})
		if appErr == nil || appErr.MessageID != "aiimage.platform.invalid" {
			t.Fatalf("expected platform %q to be rejected, got %#v", platform, appErr)
		}
		if repo.listCalls != 0 {
			t.Fatalf("invalid platform %q reached repository", platform)
		}
	}
}

func TestDeleteUsesPlatformFilter(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task}
	service := NewService(Dependencies{Repository: repo, Secretbox: box})

	appErr := service.Delete(context.Background(), task.UserID, task.ID, enum.PlatformAdmin)

	if appErr != nil {
		t.Fatalf("delete returned error: %#v", appErr)
	}
	if repo.deletePlatform != enum.PlatformAdmin {
		t.Fatalf("delete did not include platform filter: %q", repo.deletePlatform)
	}
}

func TestExecuteGeneratePropagatesQueueCancellationToExecutor(t *testing.T) {
	task := validPendingTask()
	task.RunID = 600
	task.RequestID = "image-request-600"
	task.RequestFingerprint = bytes.Repeat([]byte{1}, 32)
	task.RequestIdentityStatus = string(requestidentity.IdentityStatusReplayable)
	repo := &fakeImageRepository{task: &task, leaseClaim: true}
	executor := &fakeImageTaskExecutor{status: StatusSuccess}
	service := NewService(Dependencies{Repository: repo, Executor: executor, Now: fixedImageNow()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.ExecuteGenerate(ctx, GenerateInput{Platform: enum.PlatformAdmin, TaskID: task.ID, UserID: task.UserID})

	if !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if executor.taskID != task.ID || !executor.ctxCanceled {
		t.Fatalf("executor task=%d ctxCanceled=%v", executor.taskID, executor.ctxCanceled)
	}
}

func TestExecuteGenerateLeaseLossCancelsProviderExecutor(t *testing.T) {
	task := validPendingTask()
	task.RunID = 601
	task.RequestID = "image-request-601"
	task.RequestFingerprint = bytes.Repeat([]byte{2}, 32)
	task.RequestIdentityStatus = string(requestidentity.IdentityStatusReplayable)
	repo := &fakeImageRepository{task: &task, leaseClaim: true, renewLost: true}
	executor := &fakeImageTaskExecutor{block: true}
	service := NewService(Dependencies{Repository: repo, Executor: executor, Now: time.Now, LeaseTTL: 3 * time.Millisecond})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{Platform: enum.PlatformAdmin, TaskID: task.ID, UserID: task.UserID})

	if result != nil || !errors.Is(err, ErrTaskLeaseLost) || !executor.ctxCanceled {
		t.Fatalf("result=%#v err=%v executor=%#v", result, err, executor)
	}
}

func testImageSecretBox() secretbox.Box {
	return secretbox.New([]byte("12345678901234567890123456789012"))
}

func testUploadConfig(t *testing.T, box secretbox.Box) *UploadConfig {
	t.Helper()
	secretID, err := box.Encrypt("cos-secret-id")
	if err != nil {
		t.Fatalf("encrypt cos secret id: %v", err)
	}
	secretKey, err := box.Encrypt("cos-secret-key")
	if err != nil {
		t.Fatalf("encrypt cos secret key: %v", err)
	}
	return &UploadConfig{Driver: StorageProviderCOS, SecretIDEnc: secretID, SecretKeyEnc: secretKey, Bucket: "bucket-1250000000", Region: "ap-guangzhou", BucketDomain: "https://cos.test"}
}

func testPNGBase64(t *testing.T, width int, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0x44, G: 0x88, B: 0xcc, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func fixedImageRandom(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i + 1)
	}
	return len(p), nil
}

func validImageAgent(t *testing.T, box secretbox.Box) *AgentRuntime {
	t.Helper()
	apiKey, err := box.Encrypt("sk-test")
	if err != nil {
		t.Fatalf("encrypt api key: %v", err)
	}
	return &AgentRuntime{AgentID: 1, AgentName: "图片助手", ScenesJSON: `["image_generate"]`, AgentStatus: enum.CommonYes, ProviderID: 8, ProviderName: "OpenAI", EngineType: string(infraai.EngineTypeOpenAI), BaseURL: "https://api.openai.test/v1", APIKeyEnc: apiKey, ProviderStatus: enum.CommonYes, ModelID: RequiredModelID, ModelDisplayName: "GPT Image 2", ModelStatus: enum.CommonYes, BillingMultiplierPPM: 1_000_000, MaxOutputTokens: 32_768}
}

func validPendingTask() ImageTask {
	return ImageTask{ID: 88, Platform: enum.PlatformAdmin, UserID: 9, AgentID: 1, AgentNameSnapshot: "图片助手", ProviderIDSnapshot: 8, ProviderNameSnapshot: "OpenAI", ModelIDSnapshot: RequiredModelID, ModelDisplayNameSnapshot: "GPT Image 2", Prompt: "draw a cat", Size: defaultSize, Quality: defaultQuality, OutputFormat: defaultOutputFormat, Moderation: defaultModeration, N: defaultN, Status: StatusPending, IsFavorite: enum.CommonNo, CreatedAt: time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)}
}

func fixedImageNow() func() time.Time {
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}
