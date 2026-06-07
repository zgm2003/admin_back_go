package aiimage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/taskqueue"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type fakeImageRepository struct {
	agent            *AgentRuntime
	task             *ImageTask
	files            []ImageFile
	createdTask      ImageTask
	createdFiles     TaskFileSet
	appendedFiles    []ImageFile
	claimTask        bool
	nextTaskID       uint64
	favoriteID       uint64
	favoritePlatform string
	favoriteFlag     int
	deletePlatform   string
	failedID         uint64
	failedMsg        string
	successID        uint64
	successJSON      *string
	successRaw       *string
	listAgentsScene  string
}

func (f *fakeImageRepository) ListImageAgents(_ context.Context, scene string) ([]AgentOption, error) {
	f.listAgentsScene = scene
	return nil, nil
}
func (f *fakeImageRepository) ListTasks(context.Context, uint64, ListQuery) ([]ImageTask, int64, error) {
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
func (f *fakeImageRepository) GetTaskForWorker(ctx context.Context, userID uint64, taskID uint64) (*ImageTask, error) {
	return f.GetTask(ctx, userID, taskID, "")
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
func (f *fakeImageRepository) UpdateFavorite(_ context.Context, _ uint64, taskID uint64, platform string, isFavorite int) error {
	f.favoriteID = taskID
	f.favoritePlatform = platform
	f.favoriteFlag = isFavorite
	return nil
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
func (f *fakeImageRepository) ClaimTask(context.Context, uint64, uint64, time.Time) (bool, error) {
	return f.claimTask, nil
}
func (f *fakeImageRepository) AppendTaskFiles(_ context.Context, files []ImageFile) error {
	f.appendedFiles = append(f.appendedFiles, files...)
	return nil
}
func (f *fakeImageRepository) FinishTaskSuccess(_ context.Context, _ uint64, taskID uint64, actual *string, raw *string, _ int, _ time.Time) error {
	f.successID = taskID
	f.successJSON = actual
	f.successRaw = raw
	return nil
}
func (f *fakeImageRepository) FinishTaskFailed(_ context.Context, _ uint64, taskID uint64, message string, _ int, _ time.Time) error {
	f.failedID = taskID
	f.failedMsg = message
	return nil
}
func (f *fakeImageRepository) LoadUploadConfig(context.Context) (*UploadConfig, error) {
	return nil, nil
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

func TestPageInitListsAdminImageGenerateAgentsOnly(t *testing.T) {
	repo := &fakeImageRepository{}
	service := NewService(Dependencies{Repository: repo})

	_, appErr := service.PageInit(context.Background())

	if appErr != nil {
		t.Fatalf("expected page init to pass, got %#v", appErr)
	}
	if repo.listAgentsScene != SceneImageGenerate {
		t.Fatalf("admin image page-init must list image_generate agents only, got %q", repo.listAgentsScene)
	}
}

func TestCreateEnqueuesAdminTaskWithTaskOwnedFiles(t *testing.T) {
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
		t.Fatalf("admin task snapshot mismatch: %#v", repo.createdTask)
	}
	if len(repo.createdFiles.Inputs) != 1 || repo.createdFiles.Inputs[0].Role != FileRoleInput {
		t.Fatalf("input file was not task-owned: %#v", repo.createdFiles.Inputs)
	}
	if repo.createdFiles.Mask == nil || repo.createdFiles.Mask.File.Role != FileRoleMask || repo.createdFiles.Mask.RelatedSortOrder != 1 {
		t.Fatalf("mask file was not task-owned: %#v", repo.createdFiles.Mask)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type != TypeGenerateV1 {
		t.Fatalf("expected one admin image queue task, got %#v", enqueuer.tasks)
	}
}

func TestCreateRejectsCanvasOnlyAgentForAdminImageScene(t *testing.T) {
	box := testImageSecretBox()
	agent := validImageAgent(t, box)
	agent.ScenesJSON = `["canvas_image_generate"]`
	repo := &fakeImageRepository{agent: agent}
	service := NewService(Dependencies{Repository: repo, Enqueuer: &fakeImageEnqueuer{}, Secretbox: box})

	_, appErr := service.Create(context.Background(), CreateInput{UserID: 9, AgentID: 1, Platform: enum.PlatformAdmin, Prompt: "draw"})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "智能体未启用图片生成场景" {
		t.Fatalf("expected admin image scene gate error, got %#v", appErr)
	}
	if repo.createdTask.ID != 0 {
		t.Fatalf("admin task must not be created with a canvas-only image agent: %#v", repo.createdTask)
	}
}

func TestFavoriteUpdatesAdminTaskOnly(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task}
	service := NewService(Dependencies{Repository: repo, Secretbox: box})

	_, appErr := service.Favorite(context.Background(), FavoriteInput{UserID: task.UserID, TaskID: task.ID, Platform: enum.PlatformAdmin, IsFavorite: enum.CommonYes})

	if appErr != nil {
		t.Fatalf("favorite returned error: %#v", appErr)
	}
	if repo.favoriteID != task.ID || repo.favoritePlatform != enum.PlatformAdmin || repo.favoriteFlag != enum.CommonYes {
		t.Fatalf("favorite did not target admin task: id=%d platform=%q value=%d", repo.favoriteID, repo.favoritePlatform, repo.favoriteFlag)
	}
}

func TestDetailRejectsCrossPlatformTask(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task}
	service := NewService(Dependencies{Repository: repo, Secretbox: box})

	_, appErr := service.Detail(context.Background(), task.UserID, task.ID, enum.PlatformCanvas)

	if appErr == nil || appErr.MessageID != "aiimage.task.not_found" {
		t.Fatalf("expected cross-platform detail to be hidden, got %#v", appErr)
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

func TestExecuteGenerateRecordsAdminImageRun(t *testing.T) {
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

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{TaskID: task.ID, UserID: task.UserID})

	if err != nil || result == nil || result.Status != StatusSuccess {
		t.Fatalf("expected success, result=%#v err=%v", result, err)
	}
	if recorder.started.Platform != enum.PlatformAdmin || recorder.started.SourceType != enum.AIRunSourceImageTask || recorder.started.SourceID != task.ID {
		t.Fatalf("admin image run source mismatch: %#v", recorder.started)
	}
	if recorder.completed.RunID != 600 || recorder.completed.UsageStatus != enum.AIRunUsageReported {
		t.Fatalf("image run not completed with provider usage: %#v", recorder.completed)
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

func TestExecuteGenerateMarksTaskFailedWithoutNilResult(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	engine := &fakeImageEngine{err: errors.New("upstream boom")}
	factory := &fakeImageEngineFactory{engine: engine}
	repo := &fakeImageRepository{agent: validImageAgent(t, box), task: &task, claimTask: true}
	service := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: factory, RunRecorder: &recordingRunRecorder{}, Now: fixedImageNow()})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{TaskID: task.ID, UserID: task.UserID})

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
