package aiimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"admin_back_go/internal/module/ai/capability"
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
	nextTaskID      uint64
	deletePlatform  string
	failedID        uint64
	failedMsg       string
	successID       uint64
	successJSON     *string
	successRaw      *string
	listAgentsScene string
	listCalls       int
	workerPlatform  string
	uploadConfig    *UploadConfig
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
func (f *fakeImageRepository) DeleteTask(_ context.Context, _ uint64, _ uint64, platform string) error {
	f.deletePlatform = platform
	return nil
}
func (f *fakeImageRepository) LoadAgentRuntime(_ context.Context, agentID uint64) (*AgentRuntime, error) {
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

type fakeObjectWriter struct{ input storagecos.PutInput }

func (f *fakeObjectWriter) Put(_ context.Context, input storagecos.PutInput) error {
	f.input = input
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

func TestCreateEnqueuesGenerationTaskWithTaskOwnedFiles(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), nextTaskID: 77}
	enqueuer := &fakeImageEnqueuer{}
	service := NewService(Dependencies{Repository: repo, Enqueuer: enqueuer, Secretbox: box, Now: fixedImageNow()})

	result, appErr := service.Create(context.Background(), CreateInput{
		UserID: 9, AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "  draw a cat  ",
		InputFiles: []ImageFileInput{{StorageProvider: StorageProviderCOS, StorageKey: "inputs/ref.png", StorageURL: "https://cos.test/ref.png", MimeType: "image/png"}},
		MaskFile:   &MaskFileInput{ImageFileInput: ImageFileInput{StorageProvider: StorageProviderCOS, StorageKey: "inputs/mask.png", StorageURL: "https://cos.test/mask.png", MimeType: "image/png"}, RelatedSortOrder: 1},
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
				UserID: 9, AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "draw", Size: size,
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

func TestCreateRejectsUnregisteredImagePlatform(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{agent: validImageAgent(t, box)}
	service := NewService(Dependencies{Repository: repo, Enqueuer: &fakeImageEnqueuer{}, Secretbox: box})

	_, appErr := service.Create(context.Background(), CreateInput{UserID: 9, AgentID: 1, Platform: "partner_portal", Prompt: "draw"})

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

func TestExecuteGenerateRecordsGenerationImageRun(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	recorder := &recordingRunRecorder{}
	engine := &fakeImageEngine{result: &infraai.ImageResult{
		Images:       []infraai.GeneratedImage{{URL: "https://cdn.test/out.png", MimeType: "image/png"}},
		ActualParams: map[string]any{"size": "1024x1024"},
		RawResponse:  []byte(`{"data":[{"b64_json":"SECRET_IMAGE_BYTES"}]}`),
		UsageStatus:  infraai.UsageStatusReported,
		PromptTokens: 11, CompletionTokens: 13, TotalTokens: 24,
	}}
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task, claimTask: true}
	service := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeImageEngineFactory{engine: engine}, RunRecorder: recorder, Now: fixedImageNow()})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{Platform: enum.PlatformAdmin, TaskID: task.ID, UserID: task.UserID})

	if err != nil || result == nil || result.Status != StatusSuccess {
		t.Fatalf("expected success, result=%#v err=%v", result, err)
	}
	if recorder.started.Platform != enum.PlatformAdmin || recorder.started.RequestID != "ai_image_task_88" || recorder.started.InputSnapshot != task.Prompt {
		t.Fatalf("generation image run mismatch: %#v", recorder.started)
	}
	if repo.workerPlatform != enum.PlatformAdmin {
		t.Fatalf("worker repository calls lost platform provenance: %q", repo.workerPlatform)
	}
	if recorder.completed.RunID != 600 || recorder.completed.TotalTokens != 24 {
		t.Fatalf("image run not completed with token counts: %#v", recorder.completed)
	}
	if len(repo.appendedFiles) != 1 || repo.appendedFiles[0].Role != FileRoleOutput || repo.appendedFiles[0].StorageProvider != StorageProviderRemoteURL {
		t.Fatalf("output file mismatch: %#v", repo.appendedFiles)
	}
	if repo.successJSON == nil || !json.Valid([]byte(*repo.successJSON)) || !strings.Contains(*repo.successJSON, "1024x1024") {
		t.Fatalf("actual params were not stored as JSON: %#v", repo.successJSON)
	}
	if repo.successRaw == nil || strings.Contains(*repo.successRaw, "SECRET_IMAGE_BYTES") || !strings.Contains(*repo.successRaw, "[omitted]") {
		t.Fatalf("raw response must omit b64_json: %#v", repo.successRaw)
	}
}

func TestExecuteGeneratePersistsOutputImageDimensions(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	body := testPNGBase64(t, 2, 3)
	writer := &fakeObjectWriter{}
	engine := &fakeImageEngine{result: &infraai.ImageResult{
		Images:      []infraai.GeneratedImage{{B64JSON: body, MimeType: "image/png"}},
		UsageStatus: infraai.UsageStatusUnavailable,
	}}
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task, claimTask: true, uploadConfig: testUploadConfig(t, box)}
	service := NewService(Dependencies{
		Repository:    repo,
		Secretbox:     box,
		EngineFactory: &fakeImageEngineFactory{engine: engine},
		ObjectWriter:  writer,
		RunRecorder:   &recordingRunRecorder{},
		Now:           fixedImageNow(),
		Random:        fixedImageRandom,
	})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{Platform: enum.PlatformAdmin, TaskID: task.ID, UserID: task.UserID})

	if err != nil || result == nil || result.Status != StatusSuccess {
		t.Fatalf("expected success, result=%#v err=%v", result, err)
	}
	if len(repo.appendedFiles) != 1 {
		t.Fatalf("expected one output file, got %#v", repo.appendedFiles)
	}
	output := repo.appendedFiles[0]
	if output.Width != 2 || output.Height != 3 || output.SizeBytes <= 0 {
		t.Fatalf("output file must persist real image metadata, got width=%d height=%d size=%d", output.Width, output.Height, output.SizeBytes)
	}
	if len(writer.input.Body) == 0 || writer.input.ContentType != "image/png" {
		t.Fatalf("generated image body was not uploaded: %#v", writer.input)
	}
}

func TestExecuteGenerateMarksTaskFailedWithoutNilResult(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	engine := &fakeImageEngine{err: errors.New("upstream boom")}
	factory := &fakeImageEngineFactory{engine: engine}
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task, claimTask: true}
	service := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: factory, RunRecorder: &recordingRunRecorder{}, Now: fixedImageNow()})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{Platform: enum.PlatformAdmin, TaskID: task.ID, UserID: task.UserID})

	if err != nil {
		t.Fatalf("expected persisted provider failure to be non-retryable, got %v", err)
	}
	if result == nil || result.Status != StatusFailed || repo.failedID != task.ID || !strings.Contains(repo.failedMsg, "upstream boom") {
		t.Fatalf("failure was not persisted correctly: result=%#v id=%d message=%q", result, repo.failedID, repo.failedMsg)
	}
	if factory.config.APIKey != "sk-test" || engine.input.Model != RequiredModelID {
		t.Fatalf("engine config/input mismatch: config=%#v input=%#v", factory.config, engine.input)
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
	return &AgentRuntime{AgentID: 1, AgentName: "图片助手", ScenesJSON: `["image_generate"]`, AgentStatus: enum.CommonYes, ProviderID: 8, ProviderName: "OpenAI", EngineType: string(infraai.EngineTypeOpenAI), BaseURL: "https://api.openai.test/v1", APIKeyEnc: apiKey, ProviderStatus: enum.CommonYes, ModelID: RequiredModelID, ModelDisplayName: "GPT Image 2", ModelStatus: enum.CommonYes}
}

func validPendingTask() ImageTask {
	return ImageTask{ID: 88, Platform: enum.PlatformAdmin, UserID: 9, AgentID: 1, AgentNameSnapshot: "图片助手", ProviderIDSnapshot: 8, ProviderNameSnapshot: "OpenAI", ModelIDSnapshot: RequiredModelID, ModelDisplayNameSnapshot: "GPT Image 2", Prompt: "draw a cat", Size: defaultSize, Quality: defaultQuality, OutputFormat: defaultOutputFormat, Moderation: defaultModeration, N: defaultN, Status: StatusPending, IsFavorite: enum.CommonNo, CreatedAt: time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)}
}

func fixedImageNow() func() time.Time {
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}
