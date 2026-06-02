package canvas

import (
	"context"
	"testing"

	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/apperror"
)

type fakeCanvasRepository struct {
	prompts       []Prompt
	assets        []Asset
	agentsByScene map[string][]CanvasAgentOption
	createdPrompt Prompt
	createdAsset  Asset
	promptQuery   PromptListQuery
	assetQuery    AssetListQuery
	agentScenes   []string
	err           error
}

func (f *fakeCanvasRepository) ListPrompts(ctx context.Context, query PromptListQuery) ([]Prompt, int64, error) {
	f.promptQuery = query
	return f.prompts, int64(len(f.prompts)), f.err
}
func (f *fakeCanvasRepository) CreatePrompt(ctx context.Context, row Prompt) (int64, error) {
	f.createdPrompt = row
	return 1, f.err
}
func (f *fakeCanvasRepository) SoftDeletePrompt(ctx context.Context, id int64) error { return f.err }
func (f *fakeCanvasRepository) ListAssets(ctx context.Context, query AssetListQuery) ([]Asset, int64, error) {
	f.assetQuery = query
	return f.assets, int64(len(f.assets)), f.err
}
func (f *fakeCanvasRepository) CreateAsset(ctx context.Context, row Asset) (int64, error) {
	f.createdAsset = row
	return 2, f.err
}
func (f *fakeCanvasRepository) SoftDeleteAsset(ctx context.Context, id int64) error { return f.err }
func (f *fakeCanvasRepository) ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error) {
	f.agentScenes = append(f.agentScenes, scene)
	if f.agentsByScene == nil {
		return nil, f.err
	}
	return f.agentsByScene[scene], f.err
}

func TestServiceValidatesPromptCreate(t *testing.T) {
	svc := NewService(&fakeCanvasRepository{})
	for _, input := range []PromptInput{{Slug: "", Title: "T", Prompt: "P"}, {Slug: "s", Title: "", Prompt: "P"}, {Slug: "s", Title: "T", Prompt: ""}} {
		if _, appErr := svc.CreatePrompt(context.Background(), input); appErr == nil || appErr.Code != 100 {
			t.Fatalf("expected prompt validation error for %#v, got %#v", input, appErr)
		}
	}
}

func TestServiceValidatesAssetCreate(t *testing.T) {
	svc := NewService(&fakeCanvasRepository{})
	for _, input := range []AssetInput{{Slug: "", Type: AssetTypeText, Title: "T"}, {Slug: "s", Type: "", Title: "T"}, {Slug: "s", Type: AssetTypeText, Title: ""}, {Slug: "s", Type: "video", Title: "T"}} {
		if _, appErr := svc.CreateAsset(context.Background(), input); appErr == nil || appErr.Code != 100 {
			t.Fatalf("expected asset validation error for %#v, got %#v", input, appErr)
		}
	}
}

func TestServicePublicListsForceEnabledActiveRows(t *testing.T) {
	repo := &fakeCanvasRepository{
		prompts: []Prompt{{ID: 1, Slug: "p", Title: "Prompt", Prompt: "draw", Status: StatusEnabled, IsDel: IsDelActive}},
		assets:  []Asset{{ID: 2, Slug: "a", Type: AssetTypeImage, Title: "Asset", Status: StatusEnabled, IsDel: IsDelActive}},
	}
	svc := NewService(repo)
	prompts, appErr := svc.PublicPrompts(context.Background(), PromptListQuery{Status: StatusDisabled, IsDel: IsDelDeleted})
	if appErr != nil || len(prompts.List) != 1 || repo.promptQuery.Status != StatusEnabled || repo.promptQuery.IsDel != IsDelActive {
		t.Fatalf("public prompts mismatch resp=%#v query=%#v err=%#v", prompts, repo.promptQuery, appErr)
	}
	assets, appErr := svc.PublicAssets(context.Background(), AssetListQuery{Status: StatusDisabled, IsDel: IsDelDeleted})
	if appErr != nil || len(assets.List) != 1 || repo.assetQuery.Status != StatusEnabled || repo.assetQuery.IsDel != IsDelActive {
		t.Fatalf("public assets mismatch resp=%#v query=%#v err=%#v", assets, repo.assetQuery, appErr)
	}
}

func TestServiceAdminListCanFilterDisabledRows(t *testing.T) {
	repo := &fakeCanvasRepository{prompts: []Prompt{{ID: 1, Slug: "p", Title: "Prompt", Status: StatusDisabled, IsDel: IsDelActive}}, assets: []Asset{{ID: 2, Slug: "a", Type: AssetTypeText, Title: "Asset", Status: StatusDisabled, IsDel: IsDelActive}}}
	svc := NewService(repo)
	_, appErr := svc.ListPrompts(context.Background(), PromptListQuery{Status: StatusDisabled, IsDel: IsDelActive})
	if appErr != nil || repo.promptQuery.Status != StatusDisabled {
		t.Fatalf("admin prompt list mismatch query=%#v err=%#v", repo.promptQuery, appErr)
	}
	_, appErr = svc.ListAssets(context.Background(), AssetListQuery{Status: StatusDisabled, IsDel: IsDelActive})
	if appErr != nil || repo.assetQuery.Status != StatusDisabled {
		t.Fatalf("admin asset list mismatch query=%#v err=%#v", repo.assetQuery, appErr)
	}
}

func TestServicePublicSettingsReturnsPublicPolicyAndCanvasAgentScenes(t *testing.T) {
	auth := &fakeSettingsAuthPolicy{allowRegister: true}
	repo := &fakeCanvasRepository{agentsByScene: map[string][]CanvasAgentOption{
		canvasTextAgentScene:  {{ID: 7, Name: "文本助手", ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT 4.1 Mini", Scene: canvasTextAgentScene}},
		canvasImageAgentScene: {{ID: 8, Name: "绘图助手", ModelID: "gpt-image-2", ModelDisplayName: "GPT Image", Scene: canvasImageAgentScene}},
		canvasVideoAgentScene: {{ID: 9, Name: "视频助手", ModelID: "video-model", ModelDisplayName: "Video", Scene: canvasVideoAgentScene}},
	}}
	svc := NewServiceWithSettings(repo, SettingsDependencies{AuthPolicy: auth})

	result, appErr := svc.PublicSettings(context.Background(), SettingsInput{UserID: 7})

	if appErr != nil {
		t.Fatalf("PublicSettings error=%#v", appErr)
	}
	if !result.AllowRegister || auth.platform != "canvas" {
		t.Fatalf("auth policy mismatch result=%#v platform=%q", result, auth.platform)
	}
	if len(result.Scenes) != 3 || result.Scenes[0] != canvasTextAgentScene || result.Scenes[2] != canvasVideoAgentScene {
		t.Fatalf("unexpected scenes: %#v", result.Scenes)
	}
	if len(result.Agents.Text) != 1 || result.Agents.Text[0].Scene != canvasTextAgentScene {
		t.Fatalf("text agents must come from canvas text scene, got %#v", result.Agents.Text)
	}
	if len(result.Agents.Image) != 1 || result.Agents.Image[0].Scene != canvasImageAgentScene {
		t.Fatalf("image agents must come from canvas image scene, got %#v", result.Agents.Image)
	}
	if len(result.Agents.Video) != 1 || result.Agents.Video[0].Scene != canvasVideoAgentScene {
		t.Fatalf("video agents must come from canvas video scene, got %#v", result.Agents.Video)
	}
	if len(repo.agentScenes) != 3 || repo.agentScenes[0] != canvasTextAgentScene || repo.agentScenes[1] != canvasImageAgentScene || repo.agentScenes[2] != canvasVideoAgentScene {
		t.Fatalf("settings must query canvas agent scenes, got %#v", repo.agentScenes)
	}
}

func TestServiceChatCompletionUsesCanvasTextRuntime(t *testing.T) {
	text := &fakeCanvasTextRuntime{result: &TextGenerationResponse{Content: "hello canvas"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Text: text})

	result, appErr := svc.ChatCompletion(context.Background(), ChatCompletionInput{UserID: 7, AgentID: 8, ModelID: "gpt-4.1-mini", Message: "hi"})

	if appErr != nil {
		t.Fatalf("ChatCompletion error=%#v", appErr)
	}
	if result.Content != "hello canvas" || result.Object != "chat.completion" {
		t.Fatalf("unexpected chat result: %#v", result)
	}
	if text.input.UserID != 7 || text.input.AgentID != 8 || text.input.ModelID != "gpt-4.1-mini" || text.input.Message != "hi" {
		t.Fatalf("text runtime input mismatch: %#v", text.input)
	}
}

func TestServiceChatCompletionDoesNotReturnNotImplemented(t *testing.T) {
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Text: &fakeCanvasTextRuntime{result: &TextGenerationResponse{Content: "ok"}}})

	_, appErr := svc.ChatCompletion(context.Background(), ChatCompletionInput{UserID: 7, AgentID: 8, Message: "hi"})

	if appErr != nil && appErr.MessageID == "canvas.ai.chat.not_implemented" {
		t.Fatalf("chat must not be a not-implemented stub: %#v", appErr)
	}
}

func TestServiceGenerateImageUsesCanvasUserAndCanvasSceneRuntime(t *testing.T) {
	image := &fakeCanvasImageRuntime{}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Image: image})

	result, appErr := svc.GenerateImage(context.Background(), ImageGenerationInput{UserID: 7, AgentID: 8, Prompt: "cat", N: 2})

	if appErr != nil {
		t.Fatalf("GenerateImage error=%#v", appErr)
	}
	if result.TaskID != 501 || result.Status != "pending" {
		t.Fatalf("unexpected image result: %#v", result)
	}
	if image.input.UserID != 7 || image.input.AgentID != 8 || image.input.Platform != "canvas" || image.input.Prompt != "cat" || image.input.N != 2 {
		t.Fatalf("image runtime input mismatch: %#v", image.input)
	}
}

func TestServiceGenerateVideoCreatesFreeCanvasTask(t *testing.T) {
	video := &fakeCanvasVideoRuntime{createResult: &VideoCreateResult{ID: 77, ProviderTaskID: "provider-task-1", Status: "running"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Video: video})

	result, appErr := svc.GenerateVideo(context.Background(), VideoGenerationInput{UserID: 7, AgentID: 8, ModelID: "video-model", Prompt: "clip", DurationSeconds: 4, Size: "1280x720", ResolutionName: "720p"})

	if appErr != nil {
		t.Fatalf("GenerateVideo error=%#v", appErr)
	}
	if result.ID != 77 || result.Status != "running" {
		t.Fatalf("unexpected video result: %#v", result)
	}
	if video.createInput.UserID != 7 || video.createInput.AgentID != 8 || video.createInput.ModelID != "video-model" || video.createInput.Size != "1280x720" || video.createInput.ResolutionName != "720p" {
		t.Fatalf("video provider input mismatch: %#v", video.createInput)
	}
}

func TestServiceVideoStatusUsesCanvasVideoTaskOwnership(t *testing.T) {
	video := &fakeCanvasVideoRuntime{task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", Status: "running", IsDel: IsDelActive}, statusResult: &VideoProviderStatus{Status: "completed"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Video: video})

	result, appErr := svc.VideoStatus(context.Background(), 7, 77)

	if appErr != nil {
		t.Fatalf("VideoStatus error=%#v", appErr)
	}
	if result.ID != 77 || result.Status != "completed" || video.statusInput.Task.ProviderTaskID != "provider-task-1" {
		t.Fatalf("video status mismatch result=%#v input=%#v", result, video.statusInput)
	}
}

func TestServiceVideoContentStreamsProviderContent(t *testing.T) {
	video := &fakeCanvasVideoRuntime{task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", Status: "completed", IsDel: IsDelActive}, contentBody: []byte("video"), contentType: "video/mp4"}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Video: video})

	body, contentType, appErr := svc.VideoContent(context.Background(), 7, 77)

	if appErr != nil || string(body) != "video" || contentType != "video/mp4" {
		t.Fatalf("VideoContent mismatch body=%q contentType=%q err=%#v", string(body), contentType, appErr)
	}
	if video.contentInput.Task.ProviderTaskID != "provider-task-1" {
		t.Fatalf("content input mismatch input=%#v", video.contentInput)
	}
}

type fakeSettingsAuthPolicy struct {
	allowRegister bool
	platform      string
}

func (f *fakeSettingsAuthPolicy) AllowRegister(ctx context.Context, platform string) (bool, error) {
	f.platform = platform
	return f.allowRegister, nil
}

type fakeCanvasImageRuntime struct {
	input       aiimagemodule.CreateInput
	uploadInput aiimagemodule.CreateWithUploadedAssetsInput
}

func (f *fakeCanvasImageRuntime) Create(ctx context.Context, input aiimagemodule.CreateInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	f.input = input
	return &aiimagemodule.CreateTaskResponse{Task: aiimagemodule.TaskDTO{ID: 501, Status: "pending"}}, nil
}
func (f *fakeCanvasImageRuntime) CreateWithUploadedAssets(ctx context.Context, input aiimagemodule.CreateWithUploadedAssetsInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	f.uploadInput = input
	return &aiimagemodule.CreateTaskResponse{Task: aiimagemodule.TaskDTO{ID: 501, Status: "pending"}}, nil
}
func (f *fakeCanvasImageRuntime) Detail(ctx context.Context, userID uint64, taskID uint64) (*aiimagemodule.DetailResponse, *apperror.Error) {
	return &aiimagemodule.DetailResponse{Task: aiimagemodule.TaskDTO{ID: taskID, Status: aiimagemodule.StatusSuccess}}, nil
}

type fakeCanvasTextRuntime struct {
	input  TextGenerationInput
	result *TextGenerationResponse
	err    *apperror.Error
}

func (f *fakeCanvasTextRuntime) Generate(ctx context.Context, input TextGenerationInput) (*TextGenerationResponse, *apperror.Error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type fakeCanvasVideoRuntime struct {
	createInput  VideoCreateInput
	task         *VideoTask
	createResult *VideoCreateResult
	createErr    *apperror.Error
	statusInput  VideoStatusInput
	statusResult *VideoProviderStatus
	statusErr    *apperror.Error
	contentInput VideoContentInput
	contentBody  []byte
	contentType  string
	contentErr   *apperror.Error
}

func (f *fakeCanvasVideoRuntime) Create(ctx context.Context, input VideoCreateInput) (*VideoCreateResult, *apperror.Error) {
	f.createInput = input
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResult != nil {
		return f.createResult, nil
	}
	return &VideoCreateResult{ProviderTaskID: "provider-task", Status: "pending"}, nil
}
func (f *fakeCanvasVideoRuntime) Task(ctx context.Context, userID int64, id int64) (*VideoTask, *apperror.Error) {
	return f.task, nil
}
func (f *fakeCanvasVideoRuntime) Status(ctx context.Context, input VideoStatusInput) (*VideoProviderStatus, *apperror.Error) {
	f.statusInput = input
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	if f.statusResult != nil {
		return f.statusResult, nil
	}
	return &VideoProviderStatus{Status: "running"}, nil
}
func (f *fakeCanvasVideoRuntime) Content(ctx context.Context, input VideoContentInput) ([]byte, string, *apperror.Error) {
	f.contentInput = input
	if f.contentErr != nil {
		return nil, "", f.contentErr
	}
	return f.contentBody, f.contentType, nil
}
