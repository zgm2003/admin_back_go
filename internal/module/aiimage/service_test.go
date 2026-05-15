package aiimage

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
	"admin_back_go/internal/platform/secretbox"
	"admin_back_go/internal/platform/taskqueue"
)

type fakeImageRepository struct {
	agent          *AgentRuntime
	agentOptions   []AgentOption
	tasks          []ImageTask
	task           *ImageTask
	taskAssets     []TaskAssetRow
	assets         []ImageAsset
	uploadConfig   *UploadConfig
	nextTaskID     uint64
	nextAssetID    uint64
	claimTask      bool
	createdTask    ImageTask
	createdLinks   []ImageTaskAsset
	createdAssets  []ImageAsset
	appendedLinks  []ImageTaskAsset
	favoriteTaskID uint64
	favoriteValue  int
	deletedTaskID  uint64
	failedTaskID   uint64
	failedMessage  string
	successTaskID  uint64
	successActual  *string
	successRaw     *string
}

func (f *fakeImageRepository) ListImageAgents(ctx context.Context) ([]AgentOption, error) {
	return f.agentOptions, nil
}

func (f *fakeImageRepository) ListTasks(ctx context.Context, userID uint64, query ListQuery) ([]ImageTask, int64, error) {
	return f.tasks, int64(len(f.tasks)), nil
}

func (f *fakeImageRepository) GetTask(ctx context.Context, userID uint64, taskID uint64) (*ImageTask, error) {
	if f.task != nil && f.task.UserID == userID && f.task.ID == taskID {
		row := *f.task
		return &row, nil
	}
	return nil, nil
}

func (f *fakeImageRepository) GetTaskForWorker(ctx context.Context, userID uint64, taskID uint64) (*ImageTask, error) {
	return f.GetTask(ctx, userID, taskID)
}

func (f *fakeImageRepository) LoadTaskAssets(ctx context.Context, taskID uint64) ([]TaskAssetRow, error) {
	return f.taskAssets, nil
}

func (f *fakeImageRepository) CreateAsset(ctx context.Context, row ImageAsset) (uint64, error) {
	if f.nextAssetID == 0 {
		f.nextAssetID = 1000
	}
	row.ID = f.nextAssetID
	f.nextAssetID++
	f.createdAssets = append(f.createdAssets, row)
	return row.ID, nil
}

func (f *fakeImageRepository) LoadAssetsByIDs(ctx context.Context, userID uint64, ids []uint64) ([]ImageAsset, error) {
	out := make([]ImageAsset, 0, len(f.assets))
	allowed := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	for _, row := range f.assets {
		if row.UserID != userID {
			continue
		}
		if _, ok := allowed[row.ID]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeImageRepository) CreateTaskWithAssets(ctx context.Context, task ImageTask, links []ImageTaskAsset) (uint64, error) {
	if f.nextTaskID == 0 {
		f.nextTaskID = 900
	}
	f.createdTask = task
	f.createdLinks = append([]ImageTaskAsset(nil), links...)
	return f.nextTaskID, nil
}

func (f *fakeImageRepository) UpdateFavorite(ctx context.Context, userID uint64, taskID uint64, isFavorite int) error {
	f.favoriteTaskID = taskID
	f.favoriteValue = isFavorite
	return nil
}

func (f *fakeImageRepository) SoftDeleteTask(ctx context.Context, userID uint64, taskID uint64) error {
	f.deletedTaskID = taskID
	return nil
}

func (f *fakeImageRepository) LoadAgentRuntime(ctx context.Context, agentID uint64) (*AgentRuntime, error) {
	if f.agent == nil || f.agent.AgentID != agentID {
		return nil, nil
	}
	row := *f.agent
	return &row, nil
}

func (f *fakeImageRepository) ClaimTask(ctx context.Context, userID uint64, taskID uint64, startedAt time.Time) (bool, error) {
	return f.claimTask, nil
}

func (f *fakeImageRepository) AppendTaskAssets(ctx context.Context, links []ImageTaskAsset) error {
	f.appendedLinks = append(f.appendedLinks, links...)
	return nil
}

func (f *fakeImageRepository) FinishTaskSuccess(ctx context.Context, userID uint64, taskID uint64, actualParamsJSON *string, rawResponseJSON *string, elapsedMS int, finishedAt time.Time) error {
	f.successTaskID = taskID
	f.successActual = actualParamsJSON
	f.successRaw = rawResponseJSON
	return nil
}

func (f *fakeImageRepository) FinishTaskFailed(ctx context.Context, userID uint64, taskID uint64, message string, elapsedMS int, finishedAt time.Time) error {
	f.failedTaskID = taskID
	f.failedMessage = message
	return nil
}

func (f *fakeImageRepository) LoadUploadConfig(ctx context.Context) (*UploadConfig, error) {
	return f.uploadConfig, nil
}

type fakeImageEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

func (f *fakeImageEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	if f.err != nil {
		return taskqueue.EnqueueResult{}, f.err
	}
	f.tasks = append(f.tasks, task)
	return taskqueue.EnqueueResult{ID: "task-1", Queue: task.Queue, Type: task.Type}, nil
}

type fakeImageEngineFactory struct {
	config ImageEngineConfig
	engine platformai.ImageEngine
}

func (f *fakeImageEngineFactory) NewImageEngine(config ImageEngineConfig) platformai.ImageEngine {
	f.config = config
	return f.engine
}

type fakeImageEngine struct {
	input  platformai.ImageInput
	result *platformai.ImageResult
	err    error
}

func (f *fakeImageEngine) GenerateImages(ctx context.Context, input platformai.ImageInput) (*platformai.ImageResult, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestCreateEnqueuesPendingTaskFromImageAgent(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{
		agent:      validImageAgent(t, box),
		nextTaskID: 77,
	}
	enqueuer := &fakeImageEnqueuer{}
	now := time.Date(2026, 5, 15, 9, 30, 0, 0, time.UTC)
	service := NewService(Dependencies{
		Repository: repo,
		Enqueuer:   enqueuer,
		Secretbox:  box,
		Now:        func() time.Time { return now },
	})

	result, appErr := service.Create(context.Background(), CreateInput{
		UserID:  9,
		AgentID: 1,
		Prompt:  "  draw a cat  ",
	})

	if appErr != nil {
		t.Fatalf("expected create to pass, got %#v", appErr)
	}
	if result.Task.ID != 77 || result.Task.Status != StatusPending {
		t.Fatalf("unexpected create response: %#v", result.Task)
	}
	if repo.createdTask.UserID != 9 || repo.createdTask.AgentNameSnapshot != "图片助手" || repo.createdTask.ProviderNameSnapshot != "OpenAI" {
		t.Fatalf("agent/provider snapshot was not persisted: %#v", repo.createdTask)
	}
	if repo.createdTask.ModelIDSnapshot != RequiredModelID || repo.createdTask.Prompt != "draw a cat" {
		t.Fatalf("model or prompt snapshot mismatch: %#v", repo.createdTask)
	}
	if repo.createdTask.Size != defaultSize || repo.createdTask.Quality != defaultQuality || repo.createdTask.OutputFormat != defaultOutputFormat || repo.createdTask.Moderation != defaultModeration || repo.createdTask.N != defaultN {
		t.Fatalf("defaults were not normalized: %#v", repo.createdTask)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type != TypeGenerateV1 || enqueuer.tasks[0].Queue != taskqueue.QueueLow {
		t.Fatalf("expected one low-priority image task, got %#v", enqueuer.tasks)
	}
	payload, err := DecodeGeneratePayload(enqueuer.tasks[0].Payload)
	if err != nil {
		t.Fatalf("queued payload is invalid: %v", err)
	}
	if payload.TaskID != 77 || payload.UserID != 9 {
		t.Fatalf("queued payload mismatch: %#v", payload)
	}
}

func TestCreateRejectsAgentWithoutImageScene(t *testing.T) {
	box := testImageSecretBox()
	agent := validImageAgent(t, box)
	agent.ScenesJSON = `["chat"]`
	repo := &fakeImageRepository{agent: agent}
	service := NewService(Dependencies{
		Repository: repo,
		Enqueuer:   &fakeImageEnqueuer{},
		Secretbox:  box,
	})

	_, appErr := service.Create(context.Background(), CreateInput{UserID: 9, AgentID: 1, Prompt: "draw"})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "智能体未启用图片生成场景" {
		t.Fatalf("expected image scene gate error, got %#v", appErr)
	}
	if repo.createdTask.ID != 0 {
		t.Fatalf("task must not be created when scene gate fails: %#v", repo.createdTask)
	}
}

func TestCreateRejectsForeignInputAssets(t *testing.T) {
	box := testImageSecretBox()
	repo := &fakeImageRepository{
		agent: validImageAgent(t, box),
		assets: []ImageAsset{
			{ID: 12, UserID: 100, StorageProvider: StorageProviderCOS, StorageKey: "foreign.png", StorageURL: "https://cdn.test/foreign.png", MimeType: "image/png", SourceType: SourceTypeUpload},
		},
	}
	service := NewService(Dependencies{
		Repository: repo,
		Enqueuer:   &fakeImageEnqueuer{},
		Secretbox:  box,
	})

	_, appErr := service.Create(context.Background(), CreateInput{UserID: 9, AgentID: 1, Prompt: "draw", InputAssetIDs: []uint64{12}})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "图片资产不存在或不属于当前用户" {
		t.Fatalf("expected ownership validation error, got %#v", appErr)
	}
}

func TestExecuteGenerateMarksTaskFailedWithoutNilResult(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	engine := &fakeImageEngine{err: errors.New("upstream boom")}
	factory := &fakeImageEngineFactory{engine: engine}
	repo := &fakeImageRepository{
		agent:     validImageAgent(t, box),
		task:      &task,
		claimTask: true,
	}
	service := NewService(Dependencies{
		Repository:    repo,
		Secretbox:     box,
		EngineFactory: factory,
		Now:           fixedImageNow(),
	})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{TaskID: task.ID, UserID: task.UserID})

	if err != nil {
		t.Fatalf("expected persisted provider failure to be non-retryable, got %v", err)
	}
	if result == nil || result.Status != StatusFailed || result.TaskID != task.ID {
		t.Fatalf("failed generation must return a failed result for queue logging, got %#v", result)
	}
	if repo.failedTaskID != task.ID || !strings.Contains(repo.failedMessage, "图片生成失败") || !strings.Contains(repo.failedMessage, "upstream boom") {
		t.Fatalf("failure was not persisted correctly: id=%d message=%q", repo.failedTaskID, repo.failedMessage)
	}
	if factory.config.APIKey != "sk-test" || engine.input.Model != RequiredModelID || engine.input.Prompt != task.Prompt {
		t.Fatalf("engine config/input mismatch: config=%#v input=%#v", factory.config, engine.input)
	}
}

func TestExecuteGenerateStoresRemoteOutputAndSanitizesRawResponse(t *testing.T) {
	box := testImageSecretBox()
	task := validPendingTask()
	raw := []byte(`{"data":[{"b64_json":"SECRET_IMAGE_BYTES","url":"https://cdn.test/out.png"}]}`)
	engine := &fakeImageEngine{result: &platformai.ImageResult{
		Images: []platformai.GeneratedImage{{
			URL:           "https://cdn.test/out.png",
			MimeType:      "image/png",
			RevisedPrompt: "a better cat",
		}},
		ActualParams: map[string]any{"size": "1024x1024"},
		RawResponse:  raw,
	}}
	repo := &fakeImageRepository{
		agent:       validImageAgent(t, box),
		task:        &task,
		claimTask:   true,
		nextAssetID: 500,
	}
	service := NewService(Dependencies{
		Repository:    repo,
		Secretbox:     box,
		EngineFactory: &fakeImageEngineFactory{engine: engine},
		Now:           fixedImageNow(),
	})

	result, err := service.ExecuteGenerate(context.Background(), GenerateInput{TaskID: task.ID, UserID: task.UserID})

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result == nil || result.Status != StatusSuccess || repo.successTaskID != task.ID {
		t.Fatalf("success was not finalized: result=%#v successTaskID=%d", result, repo.successTaskID)
	}
	if len(repo.createdAssets) != 1 {
		t.Fatalf("expected one persisted output asset, got %#v", repo.createdAssets)
	}
	asset := repo.createdAssets[0]
	if asset.StorageProvider != StorageProviderRemoteURL || asset.StorageURL != "https://cdn.test/out.png" || asset.SourceType != SourceTypeGenerated {
		t.Fatalf("remote output asset mismatch: %#v", asset)
	}
	if len(repo.appendedLinks) != 1 || repo.appendedLinks[0].Role != AssetRoleOutput || repo.appendedLinks[0].AssetID != 500 || repo.appendedLinks[0].RevisedPrompt == nil {
		t.Fatalf("output link mismatch: %#v", repo.appendedLinks)
	}
	if repo.successActual == nil || !json.Valid([]byte(*repo.successActual)) || !strings.Contains(*repo.successActual, "1024x1024") {
		t.Fatalf("actual params were not stored as JSON: %#v", repo.successActual)
	}
	if repo.successRaw == nil || strings.Contains(*repo.successRaw, "SECRET_IMAGE_BYTES") || !strings.Contains(*repo.successRaw, "[omitted]") {
		t.Fatalf("raw response must omit b64_json: %#v", repo.successRaw)
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
	return &AgentRuntime{
		AgentID:          1,
		AgentName:        "图片助手",
		ScenesJSON:       `["image_generate"]`,
		AgentStatus:      enum.CommonYes,
		ProviderID:       8,
		ProviderName:     "OpenAI",
		EngineType:       string(platformai.EngineTypeOpenAI),
		BaseURL:          "https://api.openai.test/v1",
		APIKeyEnc:        apiKey,
		ProviderStatus:   enum.CommonYes,
		ModelID:          RequiredModelID,
		ModelDisplayName: "GPT Image 2",
		ModelStatus:      enum.CommonYes,
	}
}

func validPendingTask() ImageTask {
	return ImageTask{
		ID:                       88,
		UserID:                   9,
		AgentID:                  1,
		AgentNameSnapshot:        "图片助手",
		ProviderIDSnapshot:       8,
		ProviderNameSnapshot:     "OpenAI",
		ModelIDSnapshot:          RequiredModelID,
		ModelDisplayNameSnapshot: "GPT Image 2",
		Prompt:                   "draw a cat",
		Size:                     defaultSize,
		Quality:                  defaultQuality,
		OutputFormat:             defaultOutputFormat,
		Moderation:               defaultModeration,
		N:                        defaultN,
		Status:                   StatusPending,
		IsDel:                    enum.CommonNo,
		CreatedAt:                time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC),
		UpdatedAt:                time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC),
	}
}

func fixedImageNow() func() time.Time {
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}
