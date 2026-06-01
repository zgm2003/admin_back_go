package canvas

import (
	"context"
	"testing"

	aibilling "admin_back_go/internal/module/ai/billing"
	aiimagemodule "admin_back_go/internal/module/ai/image"
	walletmodule "admin_back_go/internal/module/payment/wallet"
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

func TestServicePublicSettingsReturnsOnlyPublicPolicyBillingAndWallet(t *testing.T) {
	auth := &fakeSettingsAuthPolicy{allowRegister: true}
	billing := &fakeSettingsBilling{rules: map[string]*aibilling.RuleDTO{
		aibilling.SceneCanvasTextGenerate:  {Scene: aibilling.SceneCanvasTextGenerate, Unit: aibilling.UnitRequest, UnitPriceCents: 10},
		aibilling.SceneCanvasImageGenerate: {Scene: aibilling.SceneCanvasImageGenerate, Unit: aibilling.UnitImage, UnitPriceCents: 100},
		aibilling.SceneCanvasVideoGenerate: {Scene: aibilling.SceneCanvasVideoGenerate, Unit: aibilling.UnitSecond, UnitPriceCents: 50},
	}}
	wallet := &fakeSettingsWallet{summary: &walletmodule.SummaryResponse{BalanceCents: 123, BalanceText: "1.23"}}
	repo := &fakeCanvasRepository{agentsByScene: map[string][]CanvasAgentOption{
		"chat":           {{ID: 7, Name: "文本助手", ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT 4.1 Mini", Scene: "chat"}},
		"image_generate": {{ID: 8, Name: "绘图助手", ModelID: "gpt-image-2", ModelDisplayName: "GPT Image", Scene: "image_generate"}},
	}}
	svc := NewServiceWithSettings(repo, SettingsDependencies{AuthPolicy: auth, Billing: billing, Wallet: wallet})

	result, appErr := svc.PublicSettings(context.Background(), SettingsInput{UserID: 7})

	if appErr != nil {
		t.Fatalf("PublicSettings error=%#v", appErr)
	}
	if !result.AllowRegister || auth.platform != "canvas" {
		t.Fatalf("auth policy mismatch result=%#v platform=%q", result, auth.platform)
	}
	if len(result.Scenes) != 3 || result.Scenes[0] != aibilling.SceneCanvasTextGenerate || result.Scenes[2] != aibilling.SceneCanvasVideoGenerate {
		t.Fatalf("unexpected scenes: %#v", result.Scenes)
	}
	if len(result.Billing) != 3 || result.Billing[1].Scene != aibilling.SceneCanvasImageGenerate || result.Billing[1].UnitPriceCents != 100 {
		t.Fatalf("unexpected billing: %#v", result.Billing)
	}
	if len(result.Agents.Text) != 1 || result.Agents.Text[0].ID != 7 || result.Agents.Text[0].ModelID != "gpt-4.1-mini" {
		t.Fatalf("text agents must come from ai_agents chat scene, got %#v", result.Agents.Text)
	}
	if len(result.Agents.Image) != 1 || result.Agents.Image[0].ID != 8 || result.Agents.Image[0].Scene != "image_generate" {
		t.Fatalf("image agents must come from ai_agents image_generate scene, got %#v", result.Agents.Image)
	}
	if len(result.Agents.Video) != 1 || result.Agents.Video[0].ID != 8 {
		t.Fatalf("video agents must also come from configured ai_agents, got %#v", result.Agents.Video)
	}
	if len(repo.agentScenes) != 2 || repo.agentScenes[0] != "chat" || repo.agentScenes[1] != "image_generate" {
		t.Fatalf("settings must query configured agent scenes, got %#v", repo.agentScenes)
	}
	if result.Wallet == nil || result.Wallet.BalanceCents != 123 || wallet.userID != 7 {
		t.Fatalf("wallet mismatch result=%#v user=%d", result.Wallet, wallet.userID)
	}
}

func TestServicePublicSettingsDoesNotInventAgentsFromBillingScenes(t *testing.T) {
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{
		AuthPolicy: &fakeSettingsAuthPolicy{allowRegister: true},
		Billing: &fakeSettingsBilling{rules: map[string]*aibilling.RuleDTO{
			aibilling.SceneCanvasImageGenerate: {Scene: aibilling.SceneCanvasImageGenerate, Unit: aibilling.UnitImage, UnitPriceCents: 100},
		}},
	})

	result, appErr := svc.PublicSettings(context.Background(), SettingsInput{UserID: 7})

	if appErr != nil {
		t.Fatalf("PublicSettings error=%#v", appErr)
	}
	if len(result.Agents.Text) != 0 || len(result.Agents.Image) != 0 || len(result.Agents.Video) != 0 {
		t.Fatalf("billing scenes must not become selectable agents: %#v", result.Agents)
	}
}

func TestServicePublicSettingsOmitsWalletForAnonymousUser(t *testing.T) {
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{
		AuthPolicy: &fakeSettingsAuthPolicy{allowRegister: true},
		Billing: &fakeSettingsBilling{rules: map[string]*aibilling.RuleDTO{
			aibilling.SceneCanvasTextGenerate: {Scene: aibilling.SceneCanvasTextGenerate, Unit: aibilling.UnitRequest, UnitPriceCents: 10},
		}},
		Wallet: &fakeSettingsWallet{summary: &walletmodule.SummaryResponse{BalanceCents: 123}},
	})

	result, appErr := svc.PublicSettings(context.Background(), SettingsInput{})

	if appErr != nil {
		t.Fatalf("PublicSettings error=%#v", appErr)
	}
	if result.Wallet != nil {
		t.Fatalf("anonymous settings must not query/include wallet: %#v", result.Wallet)
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
	if image.input.UserID != 7 || image.input.AgentID != 8 || image.input.Platform != "canvas" || image.input.BillingScene != aibilling.SceneCanvasImageGenerate || image.input.Prompt != "cat" || image.input.N != 2 {
		t.Fatalf("image runtime input mismatch: %#v", image.input)
	}
}

func TestServiceGenerateVideoChargesBeforeProviderBindsTaskAndReturnsBillingRecordID(t *testing.T) {
	billing := &fakeSettingsBilling{chargeResult: &aibilling.ChargeResult{RecordID: 77}}
	video := &fakeCanvasVideoRuntime{createResult: &VideoCreateResult{ProviderTaskID: "provider-task-1", Status: "running"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Billing: billing, Video: video})

	result, appErr := svc.GenerateVideo(context.Background(), VideoGenerationInput{UserID: 7, AgentID: 8, ModelID: "video-model", Prompt: "clip", DurationSeconds: 4, Size: "1280x720", ResolutionName: "720p"})

	if appErr != nil {
		t.Fatalf("GenerateVideo error=%#v", appErr)
	}
	if result.ID != 77 || result.Status != "running" {
		t.Fatalf("unexpected video result: %#v", result)
	}
	if billing.chargeInput.Platform != "canvas" || billing.chargeInput.Scene != aibilling.SceneCanvasVideoGenerate || billing.chargeInput.UserID != 7 || billing.chargeInput.UnitCount != 4 {
		t.Fatalf("video charge input mismatch: %#v", billing.chargeInput)
	}
	if video.createInput.UserID != 7 || video.createInput.AgentID != 8 || video.createInput.ModelID != "video-model" || video.createInput.Size != "1280x720" || video.createInput.ResolutionName != "720p" {
		t.Fatalf("video provider input mismatch: %#v", video.createInput)
	}
	if billing.boundRecordID != 77 || billing.boundProviderTaskID != "provider-task-1" {
		t.Fatalf("provider task not bound: id=%d task=%q", billing.boundRecordID, billing.boundProviderTaskID)
	}
}

func TestServiceGenerateVideoStopsBeforeProviderOnInsufficientBalance(t *testing.T) {
	billing := &fakeSettingsBilling{chargeErr: apperror.BadRequestKey("wallet.debit.insufficient_balance", nil, "余额不足")}
	video := &fakeCanvasVideoRuntime{createResult: &VideoCreateResult{ProviderTaskID: "task"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Billing: billing, Video: video})

	result, appErr := svc.GenerateVideo(context.Background(), VideoGenerationInput{UserID: 7, AgentID: 8, Prompt: "clip", DurationSeconds: 4})

	if appErr == nil || appErr.MessageID != "wallet.debit.insufficient_balance" {
		t.Fatalf("expected insufficient balance, result=%#v err=%#v", result, appErr)
	}
	if len(billing.refundInputs) != 0 {
		t.Fatalf("must not refund when charge never succeeded: %#v", billing.refundInputs)
	}
	if video.createInput.UserID != 0 {
		t.Fatalf("provider must not be called when charge fails: %#v", video.createInput)
	}
}

func TestServiceGenerateVideoRefundsOnceWhenProviderCreateFails(t *testing.T) {
	billing := &fakeSettingsBilling{chargeResult: &aibilling.ChargeResult{RecordID: 77}}
	video := &fakeCanvasVideoRuntime{createErr: apperror.BadRequestKey("canvas.ai.video.provider_failed", nil, "provider failed")}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Billing: billing, Video: video})

	_, appErr := svc.GenerateVideo(context.Background(), VideoGenerationInput{UserID: 7, AgentID: 8, Prompt: "clip", DurationSeconds: 4})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_failed" {
		t.Fatalf("expected provider failure, got %#v", appErr)
	}
	if len(billing.refundInputs) != 1 || billing.refundInputs[0].BillingRecordID != 77 {
		t.Fatalf("expected one refund, got %#v", billing.refundInputs)
	}
}

func TestServiceVideoStatusUsesBillingRecordOwnershipContract(t *testing.T) {
	billing := &fakeSettingsBilling{record: &aibilling.BillingRecord{ID: 77, UserID: 7, Platform: "canvas", Scene: aibilling.SceneCanvasVideoGenerate, Status: aibilling.BillingStatusCharged, ProviderTaskID: "provider-task-1"}}
	video := &fakeCanvasVideoRuntime{statusResult: &VideoProviderStatus{Status: "completed"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Billing: billing, Video: video})

	result, appErr := svc.VideoStatus(context.Background(), 7, 77)

	if appErr != nil {
		t.Fatalf("VideoStatus error=%#v", appErr)
	}
	if result.ID != 77 || result.Status != "completed" || billing.recordQueryID != 77 || video.statusInput.BillingRecord.ProviderTaskID != "provider-task-1" {
		t.Fatalf("video status mismatch result=%#v recordID=%d input=%#v", result, billing.recordQueryID, video.statusInput)
	}
	if billing.markSuccessID != 77 {
		t.Fatalf("completed video must mark billing success, got %d", billing.markSuccessID)
	}
}

func TestServiceVideoStatusRefundsFailedProviderStatusOnce(t *testing.T) {
	billing := &fakeSettingsBilling{record: &aibilling.BillingRecord{ID: 77, UserID: 7, Platform: "canvas", Scene: aibilling.SceneCanvasVideoGenerate, Status: aibilling.BillingStatusCharged, ProviderTaskID: "provider-task-1"}}
	video := &fakeCanvasVideoRuntime{statusResult: &VideoProviderStatus{Status: "failed", ErrorMessage: "provider failed"}}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Billing: billing, Video: video})

	result, appErr := svc.VideoStatus(context.Background(), 7, 77)

	if appErr != nil || result.Status != "failed" {
		t.Fatalf("expected failed status without service error, result=%#v err=%#v", result, appErr)
	}
	if len(billing.refundInputs) != 1 || billing.refundInputs[0].BillingRecordID != 77 {
		t.Fatalf("expected one refund, got %#v", billing.refundInputs)
	}
}

func TestServiceVideoContentStreamsProviderContentAndMarksSuccess(t *testing.T) {
	billing := &fakeSettingsBilling{record: &aibilling.BillingRecord{ID: 77, UserID: 7, Platform: "canvas", Scene: aibilling.SceneCanvasVideoGenerate, Status: aibilling.BillingStatusCharged, ProviderTaskID: "provider-task-1"}}
	video := &fakeCanvasVideoRuntime{contentBody: []byte("video"), contentType: "video/mp4"}
	svc := NewServiceWithSettings(&fakeCanvasRepository{}, SettingsDependencies{Billing: billing, Video: video})

	body, contentType, appErr := svc.VideoContent(context.Background(), 7, 77)

	if appErr != nil || string(body) != "video" || contentType != "video/mp4" {
		t.Fatalf("VideoContent mismatch body=%q contentType=%q err=%#v", string(body), contentType, appErr)
	}
	if video.contentInput.BillingRecord.ProviderTaskID != "provider-task-1" || billing.markSuccessID != 77 {
		t.Fatalf("content input/mark success mismatch input=%#v mark=%d", video.contentInput, billing.markSuccessID)
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

type fakeSettingsBilling struct {
	rules               map[string]*aibilling.RuleDTO
	scenes              []string
	chargeInput         aibilling.ChargeInput
	chargeResult        *aibilling.ChargeResult
	chargeErr           *apperror.Error
	refundInputs        []aibilling.RefundInput
	record              *aibilling.BillingRecord
	recordQueryID       int64
	boundRecordID       int64
	boundProviderTaskID string
	markSuccessID       int64
}

func (f *fakeSettingsBilling) EnabledRule(ctx context.Context, scene string) (*aibilling.RuleDTO, *apperror.Error) {
	f.scenes = append(f.scenes, scene)
	if rule, ok := f.rules[scene]; ok {
		return rule, nil
	}
	return nil, apperror.BadRequestKey("aibilling.rule.not_configured", nil, "AI计费规则未配置或已禁用")
}
func (f *fakeSettingsBilling) Charge(ctx context.Context, input aibilling.ChargeInput) (*aibilling.ChargeResult, *apperror.Error) {
	f.chargeInput = input
	if f.chargeErr != nil {
		return nil, f.chargeErr
	}
	if f.chargeResult != nil {
		return f.chargeResult, nil
	}
	return &aibilling.ChargeResult{RecordID: 1}, nil
}
func (f *fakeSettingsBilling) Refund(ctx context.Context, input aibilling.RefundInput) *apperror.Error {
	f.refundInputs = append(f.refundInputs, input)
	return nil
}
func (f *fakeSettingsBilling) BillingRecord(ctx context.Context, id int64) (*aibilling.BillingRecord, *apperror.Error) {
	f.recordQueryID = id
	return f.record, nil
}
func (f *fakeSettingsBilling) BindProviderTask(ctx context.Context, billingRecordID int64, providerTaskID string) *apperror.Error {
	f.boundRecordID = billingRecordID
	f.boundProviderTaskID = providerTaskID
	return nil
}
func (f *fakeSettingsBilling) MarkSuccess(ctx context.Context, billingRecordID int64) *apperror.Error {
	f.markSuccessID = billingRecordID
	return nil
}

type fakeSettingsWallet struct {
	userID  int64
	summary *walletmodule.SummaryResponse
}

func (f *fakeSettingsWallet) Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error) {
	f.userID = userID
	return f.summary, nil
}

type fakeCanvasImageRuntime struct {
	input aiimagemodule.CreateInput
}

func (f *fakeCanvasImageRuntime) Create(ctx context.Context, input aiimagemodule.CreateInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	f.input = input
	return &aiimagemodule.CreateTaskResponse{Task: aiimagemodule.TaskDTO{ID: 501, Status: "pending"}}, nil
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
