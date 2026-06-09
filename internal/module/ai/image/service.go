package aiimage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/infra/taskqueue"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"

	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
)

const (
	SceneCanvasImageGenerate = "canvas_image_generate"
	RequiredModelID          = "gpt-image-2"

	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"

	FileRoleInput  = "input"
	FileRoleMask   = "mask"
	FileRoleOutput = "output"

	StorageProviderCOS       = "cos"
	StorageProviderRemoteURL = "remote_url"

	defaultSize         = "1024x1024"
	defaultQuality      = "auto"
	defaultOutputFormat = "png"
	defaultModeration   = "auto"
	defaultN            = 1
)

const serviceTimeLayout = "2006-01-02 15:04:05"

var (
	statusLabels     = map[string]string{StatusPending: "等待中", StatusRunning: "生成中", StatusSuccess: "成功", StatusFailed: "失败"}
	sizeLabels       = map[string]string{"auto": "自动", "1024x1024": "1024×1024", "1536x1024": "1536×1024", "1024x1536": "1024×1536", "1792x1024": "1792×1024", "1024x1792": "1024×1792"}
	qualityLabels    = map[string]string{"auto": "自动", "low": "低", "medium": "中", "high": "高"}
	formatLabels     = map[string]string{"png": "PNG", "jpeg": "JPEG", "webp": "WebP"}
	moderationLabels = map[string]string{"auto": "自动", "low": "低限制"}
)

type Service struct {
	repository    Repository
	enqueuer      taskqueue.Enqueuer
	secretbox     secretbox.Box
	engineFactory ImageEngineFactory
	objectReader  storagecos.ObjectReader
	objectWriter  storagecos.ObjectWriter
	runRecorder   airun.Recorder
	now           func() time.Time
	random        func([]byte) (int, error)
}

type Dependencies struct {
	Repository    Repository
	Enqueuer      taskqueue.Enqueuer
	Secretbox     secretbox.Box
	EngineFactory ImageEngineFactory
	ObjectReader  storagecos.ObjectReader
	ObjectWriter  storagecos.ObjectWriter
	RunRecorder   airun.Recorder
	Now           func() time.Time
	Random        func([]byte) (int, error)
}

type ImageEngineConfig struct {
	EngineType string
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
}

type ImageEngineFactory interface {
	NewImageEngine(config ImageEngineConfig) infraai.ImageEngine
}
type ImageEngineFactoryFunc func(config ImageEngineConfig) infraai.ImageEngine

func (f ImageEngineFactoryFunc) NewImageEngine(config ImageEngineConfig) infraai.ImageEngine {
	if f == nil {
		return nil
	}
	return f(config)
}

func NewService(deps Dependencies) *Service {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Random == nil {
		deps.Random = rand.Read
	}
	return &Service{repository: deps.Repository, enqueuer: deps.Enqueuer, secretbox: deps.Secretbox, engineFactory: deps.EngineFactory, objectReader: deps.ObjectReader, objectWriter: deps.ObjectWriter, runRecorder: deps.RunRecorder, now: deps.Now, random: deps.Random}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agents, err := repo.ListImageAgents(ctx, SceneCanvasImageGenerate)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.agent.query_failed", nil, "查询图片智能体失败", err)
	}
	if agents == nil {
		agents = []AgentOption{}
	}
	return &PageInitResponse{Dict: PageInitDict{SizeArr: stringOptions([]string{"auto", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792"}, sizeLabels), QualityArr: stringOptions([]string{"auto", "low", "medium", "high"}, qualityLabels), OutputFormatArr: stringOptions([]string{"png", "jpeg", "webp"}, formatLabels), ModerationArr: stringOptions([]string{"auto", "low"}, moderationLabels), StatusArr: stringOptions([]string{StatusPending, StatusRunning, StatusSuccess, StatusFailed}, statusLabels)}, AgentOptions: agents}, nil
}

func (s *Service) List(ctx context.Context, userID uint64, query ListQuery) (*ListResponse, *apperror.Error) {
	if userID == 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	if query.Status != "" && !isStatus(query.Status) {
		return nil, apperror.BadRequestKey("aiimage.task.status.invalid", nil, "无效的图片任务状态")
	}
	rows, total, err := repo.ListTasks(ctx, userID, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task.query_failed", nil, "查询图片任务失败", err)
	}
	list := make([]TaskDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, taskDTO(row))
	}
	return &ListResponse{List: list, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: total, TotalPage: totalPage(total, query.PageSize)}}, nil
}

func (s *Service) Detail(ctx context.Context, userID uint64, taskID uint64, platform string) (*DetailResponse, *apperror.Error) {
	if userID == 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if taskID == 0 {
		return nil, apperror.BadRequestKey("aiimage.task.id.invalid", nil, "无效的图片任务ID")
	}
	if appErr := validateTaskPlatform(platform); appErr != nil {
		return nil, appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	task, err := repo.GetTask(ctx, userID, taskID, platform)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task.query_failed", nil, "查询图片任务失败", err)
	}
	if task == nil {
		return nil, apperror.NotFoundKey("aiimage.task.not_found", nil, "图片任务不存在")
	}
	files, err := repo.LoadTaskFiles(ctx, taskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task_assets.query_failed", nil, "查询图片任务文件失败", err)
	}
	return detailResponse(*task, files), nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateTaskResponse, *apperror.Error) {
	if s == nil || s.enqueuer == nil {
		return nil, apperror.InternalKey("aiimage.queue_missing", nil, "图片生成队列未配置")
	}
	normalized, appErr := s.normalizeCreateInput(input)
	if appErr != nil {
		return nil, appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agent, appErr := s.validImageAgent(ctx, repo, normalized.AgentID, SceneCanvasImageGenerate)
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	files, appErr := buildTaskFiles(normalized, now)
	if appErr != nil {
		return nil, appErr
	}
	task := ImageTask{Platform: normalized.Platform, UserID: normalized.UserID, AgentID: normalized.AgentID, AgentNameSnapshot: agent.AgentName, ProviderIDSnapshot: agent.ProviderID, ProviderNameSnapshot: agent.ProviderName, ModelIDSnapshot: agent.ModelID, ModelDisplayNameSnapshot: agent.ModelDisplayName, Prompt: normalized.Prompt, Size: normalized.Size, Quality: normalized.Quality, OutputFormat: normalized.OutputFormat, OutputCompression: normalized.OutputCompression, Moderation: normalized.Moderation, N: normalized.N, Status: StatusPending, IsFavorite: enum.CommonNo, CreatedAt: now, UpdatedAt: now}
	id, err := repo.CreateTaskWithFiles(ctx, task, files)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task.create_failed", nil, "创建图片任务失败", err)
	}
	task.ID = id
	queueTask, err := NewGenerateTask(GeneratePayload{TaskID: id, UserID: normalized.UserID})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.queue_task.create_failed", nil, "创建图片队列任务失败", err)
	}
	if _, err := s.enqueuer.Enqueue(ctx, queueTask); err != nil {
		_ = repo.FinishTaskFailed(context.Background(), normalized.UserID, id, "图片生成任务入队失败", 0, s.now())
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.queue_task.enqueue_failed", nil, "图片生成任务入队失败", err)
	}
	return &CreateTaskResponse{Task: taskDTO(task)}, nil
}

func (s *Service) CreateWithUploadedFiles(ctx context.Context, input CreateWithUploadedFilesInput) (*CreateTaskResponse, *apperror.Error) {
	if len(input.Files) == 0 {
		return s.Create(ctx, input.CreateInput)
	}
	uploaded, appErr := s.storeUploadedFiles(ctx, input.CreateInput.UserID, input.Files)
	if appErr != nil {
		return nil, appErr
	}
	input.CreateInput.InputFiles = append(input.CreateInput.InputFiles, uploaded...)
	return s.Create(ctx, input.CreateInput)
}

func (s *Service) Delete(ctx context.Context, userID uint64, taskID uint64, platform string) *apperror.Error {
	if userID == 0 {
		return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if taskID == 0 {
		return apperror.BadRequestKey("aiimage.task.id.invalid", nil, "无效的图片任务ID")
	}
	if appErr := validateTaskPlatform(platform); appErr != nil {
		return appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.DeleteTask(ctx, userID, taskID, platform); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFoundKey("aiimage.task.not_found", nil, "图片任务不存在")
		}
		return apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task.delete_failed", nil, "删除图片任务失败", err)
	}
	return nil
}

func (s *Service) ExecuteGenerate(ctx context.Context, input GenerateInput) (*GenerateResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	startedAt := s.now()
	claimed, err := repo.ClaimTask(ctx, input.UserID, input.TaskID, startedAt)
	if err != nil {
		return nil, fmt.Errorf("claim ai image task: %w", err)
	}
	if !claimed {
		task, loadErr := repo.GetTaskForWorker(ctx, input.UserID, input.TaskID)
		if loadErr != nil {
			return nil, fmt.Errorf("load unclaimed ai image task: %w", loadErr)
		}
		status := "unclaimed"
		if task != nil {
			status = task.Status
		}
		return &GenerateResult{TaskID: input.TaskID, Status: status}, nil
	}
	task, err := repo.GetTaskForWorker(ctx, input.UserID, input.TaskID)
	if err != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "读取图片任务失败", err)
	}
	if task == nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "图片任务不存在", nil)
	}
	if appErr := validateTaskPlatform(task.Platform); appErr != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, appErr.Message, appErr)
	}
	if s.runRecorder == nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "图片运行记录服务未配置", nil)
	}
	runID, err := s.runRecorder.Start(ctx, airun.StartInput{Platform: task.Platform, RequestID: fmt.Sprintf("ai_image_task_%d", task.ID), UserID: int64(task.UserID), AgentID: int64(task.AgentID), ProviderID: int64(task.ProviderIDSnapshot), ModelID: task.ModelIDSnapshot, ModelDisplayName: task.ModelDisplayNameSnapshot, InputSnapshot: task.Prompt, StartedAt: startedAt})
	if err != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "创建图片运行记录失败", err)
	}
	agent, appErr := s.validImageAgent(ctx, repo, task.AgentID, SceneCanvasImageGenerate)
	if appErr != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, appErr.Message, appErr)
	}
	files, err := repo.LoadTaskFiles(ctx, task.ID)
	if err != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, "读取图片任务文件失败", err)
	}
	assets, appErr := s.engineAssets(ctx, repo, files)
	if appErr != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, appErr.Message, appErr)
	}
	apiKey, appErr := s.decryptProviderKey(agent.APIKeyEnc)
	if appErr != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, appErr.Message, appErr)
	}
	engine := s.imageEngine(ImageEngineConfig{EngineType: agent.EngineType, BaseURL: agent.BaseURL, APIKey: apiKey, Timeout: 10 * time.Minute})
	if engine == nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, "图片生成引擎未配置", nil)
	}
	result, err := engine.GenerateImages(ctx, infraai.ImageInput{Model: task.ModelIDSnapshot, Prompt: task.Prompt, Size: task.Size, Quality: task.Quality, OutputFormat: task.OutputFormat, OutputCompression: task.OutputCompression, Moderation: task.Moderation, N: task.N, InputAssets: assets.inputs, MaskAsset: assets.mask})
	if err != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, "图片生成失败", err)
	}
	if result == nil || len(result.Images) == 0 {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, "图片生成结果为空", nil)
	}
	if appErr := validateImageRunUsageStatus(result); appErr != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, appErr.Message, appErr)
	}
	if appErr := s.persistOutputs(ctx, repo, *task, result); appErr != nil {
		return s.finishGenerateFailedWithRun(context.Background(), repo, input, runID, startedAt, appErr.Message, appErr)
	}
	finishedAt := s.now()
	actualParamsJSON := jsonString(result.ActualParams)
	rawResponseJSON := sanitizeRawResponse(result.RawResponse)
	if err := repo.FinishTaskSuccess(ctx, task.UserID, task.ID, actualParamsJSON, rawResponseJSON, elapsedMS(startedAt, finishedAt), finishedAt); err != nil {
		return nil, fmt.Errorf("finish ai image task success: %w", err)
	}
	if err := s.runRecorder.Complete(context.Background(), airun.CompleteInput{RunID: runID, PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, TotalTokens: result.TotalTokens, FinishedAt: finishedAt, DurationMS: uint(elapsedMS(startedAt, finishedAt))}); err != nil {
		return nil, fmt.Errorf("finish ai image run success: %w", err)
	}
	return &GenerateResult{TaskID: task.ID, Status: StatusSuccess}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("aiimage.repository_missing", nil, "AI图片仓储未配置")
	}
	return s.repository, nil
}

func validateTaskPlatform(platform string) *apperror.Error {
	if platform != enum.PlatformCanvas {
		return apperror.BadRequestKey("aiimage.platform.invalid", nil, "无效的图片任务平台")
	}
	return nil
}

func (s *Service) normalizeCreateInput(input CreateInput) (CreateInput, *apperror.Error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Platform = strings.TrimSpace(input.Platform)
	input.Size = strings.TrimSpace(input.Size)
	input.Quality = strings.TrimSpace(input.Quality)
	input.OutputFormat = strings.ToLower(strings.TrimSpace(input.OutputFormat))
	input.Moderation = strings.TrimSpace(input.Moderation)
	if input.UserID == 0 {
		return CreateInput{}, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.AgentID == 0 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.agent.required", nil, "图片智能体不能为空")
	}
	if input.Prompt == "" {
		return CreateInput{}, apperror.BadRequestKey("aiimage.prompt.required", nil, "提示词不能为空")
	}
	if input.Platform != enum.PlatformCanvas {
		return CreateInput{}, apperror.BadRequestKey("aiimage.platform.invalid", nil, "无效的图片任务平台")
	}
	if len([]rune(input.Prompt)) > 20000 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.prompt.too_long", nil, "提示词不能超过20000个字符")
	}
	if input.Size == "" {
		input.Size = defaultSize
	}
	if !knownValue(input.Size, sizeLabels) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.size.invalid", nil, "无效的图片尺寸")
	}
	if input.Quality == "" {
		input.Quality = defaultQuality
	}
	if !knownValue(input.Quality, qualityLabels) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.quality.invalid", nil, "无效的图片质量")
	}
	if input.OutputFormat == "" {
		input.OutputFormat = defaultOutputFormat
	}
	if !knownValue(input.OutputFormat, formatLabels) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.output_format.invalid", nil, "无效的输出格式")
	}
	if input.OutputCompression != nil && (*input.OutputCompression < 0 || *input.OutputCompression > 100) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.output_compression.invalid", nil, "输出压缩率必须在0到100之间")
	}
	if input.Moderation == "" {
		input.Moderation = defaultModeration
	}
	if !knownValue(input.Moderation, moderationLabels) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.moderation.invalid", nil, "无效的审核参数")
	}
	if input.N <= 0 {
		input.N = defaultN
	}
	maxN := 4
	if input.Platform == enum.PlatformCanvas {
		maxN = 15
	}
	if input.N > maxN {
		return CreateInput{}, apperror.BadRequestKey("aiimage.n.too_many", nil, "单次生成图片数量超出限制")
	}
	if len(input.InputFiles) > 10 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.input_assets.too_many", nil, "参考图最多10张")
	}
	if input.MaskFile != nil && len(input.InputFiles) == 0 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.mask.requires_input", nil, "遮罩图必须配合参考图使用")
	}
	if input.MaskFile != nil && (input.MaskFile.RelatedSortOrder <= 0 || input.MaskFile.RelatedSortOrder > len(input.InputFiles)) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.mask_target.invalid", nil, "遮罩目标图必须在参考图中")
	}
	return input, nil
}

func buildTaskFiles(input CreateInput, now time.Time) (TaskFileSet, *apperror.Error) {
	out := TaskFileSet{Inputs: make([]ImageFile, 0, len(input.InputFiles))}
	for index, fileInput := range input.InputFiles {
		file, appErr := normalizeFileInput(fileInput, FileRoleInput, index+1, now)
		if appErr != nil {
			return TaskFileSet{}, appErr
		}
		out.Inputs = append(out.Inputs, file)
	}
	if input.MaskFile != nil {
		file, appErr := normalizeFileInput(input.MaskFile.ImageFileInput, FileRoleMask, 1, now)
		if appErr != nil {
			return TaskFileSet{}, appErr
		}
		out.Mask = &MaskImageFile{File: file, RelatedSortOrder: input.MaskFile.RelatedSortOrder}
	}
	return out, nil
}

func normalizeFileInput(input ImageFileInput, role string, sortOrder int, now time.Time) (ImageFile, *apperror.Error) {
	provider := strings.TrimSpace(input.StorageProvider)
	key := strings.TrimSpace(input.StorageKey)
	urlValue := strings.TrimSpace(input.StorageURL)
	mimeType := strings.TrimSpace(input.MimeType)
	if provider != StorageProviderCOS && provider != StorageProviderRemoteURL {
		return ImageFile{}, apperror.BadRequestKey("aiimage.asset.storage_provider.unsupported", nil, "不支持的图片存储类型")
	}
	if provider == StorageProviderCOS && key == "" {
		return ImageFile{}, apperror.BadRequestKey("aiimage.asset.cos_key.required", nil, "COS图片key不能为空")
	}
	if urlValue == "" {
		return ImageFile{}, apperror.BadRequestKey("aiimage.asset.url.required", nil, "图片URL不能为空")
	}
	if !validURL(urlValue) {
		return ImageFile{}, apperror.BadRequestKey("aiimage.asset.url.invalid", nil, "图片URL不合法")
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return ImageFile{}, apperror.BadRequestKey("aiimage.asset.mime.invalid", nil, "图片MIME类型不合法")
	}
	return ImageFile{Role: role, SortOrder: sortOrder, StorageProvider: provider, StorageKey: key, StorageURL: urlValue, MimeType: mimeType, Width: input.Width, Height: input.Height, SizeBytes: input.SizeBytes, CreatedAt: now}, nil
}

func (s *Service) validImageAgent(ctx context.Context, repo Repository, agentID uint64, expectedScene string) (*AgentRuntime, *apperror.Error) {
	agent, err := repo.LoadAgentRuntime(ctx, agentID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.agent.query_failed", nil, "查询图片智能体失败", err)
	}
	if agent == nil {
		return nil, apperror.NotFoundKey("aiimage.agent.not_found", nil, "图片智能体不存在或未启用")
	}
	if agent.AgentStatus != enum.CommonYes || agent.ProviderStatus != enum.CommonYes || agent.ModelStatus != enum.CommonYes {
		return nil, apperror.BadRequestKey("aiimage.agent.runtime_disabled", nil, "图片智能体、供应商或模型未启用")
	}
	if expectedScene != "" && !sceneEnabled(agent.ScenesJSON, expectedScene) {
		return nil, apperror.BadRequestKey("aiimage.agent.scene_missing", nil, "智能体未启用图片生成场景")
	}
	if agent.ModelID != RequiredModelID {
		return nil, apperror.BadRequestKey("aiimage.model.unsupported", nil, "Canvas图片生成只支持 gpt-image-2")
	}
	if strings.TrimSpace(agent.APIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("aiimage.provider.api_key_missing", nil, "AI供应商API Key未配置")
	}
	if strings.TrimSpace(agent.EngineType) != string(infraai.EngineTypeOpenAI) {
		return nil, apperror.BadRequestKey("aiimage.provider.unsupported", nil, "Canvas图片生成只支持 OpenAI-compatible 供应商")
	}
	return agent, nil
}

type preparedEngineFiles struct {
	inputs []infraai.ImageAsset
	mask   *infraai.ImageAsset
}

func (s *Service) engineAssets(ctx context.Context, repo Repository, rows []ImageFile) (*preparedEngineFiles, *apperror.Error) {
	var inputRows []ImageFile
	var maskRow *ImageFile
	for _, row := range rows {
		switch row.Role {
		case FileRoleInput:
			inputRows = append(inputRows, row)
		case FileRoleMask:
			copyRow := row
			maskRow = &copyRow
		}
	}
	if len(inputRows) == 0 && maskRow == nil {
		return &preparedEngineFiles{}, nil
	}
	cfg, appErr := s.loadCOSConfig(ctx, repo)
	if appErr != nil {
		return nil, appErr
	}
	out := &preparedEngineFiles{inputs: make([]infraai.ImageAsset, 0, len(inputRows))}
	for _, row := range inputRows {
		asset, appErr := s.readCOSFile(ctx, cfg, row)
		if appErr != nil {
			return nil, appErr
		}
		out.inputs = append(out.inputs, asset)
	}
	if maskRow != nil {
		asset, appErr := s.readCOSFile(ctx, cfg, *maskRow)
		if appErr != nil {
			return nil, appErr
		}
		out.mask = &asset
	}
	return out, nil
}

func (s *Service) loadCOSConfig(ctx context.Context, repo Repository) (*cosRuntimeConfig, *apperror.Error) {
	cfg, err := repo.LoadUploadConfig(ctx)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.upload_config.read_failed", nil, "读取上传配置失败", err)
	}
	if cfg == nil || cfg.Driver != StorageProviderCOS {
		return nil, apperror.InternalKey("aiimage.cos_config.missing", nil, "未配置有效的 COS 上传配置")
	}
	secretID, err := s.secretbox.Decrypt(cfg.SecretIDEnc)
	if err != nil || strings.TrimSpace(secretID) == "" {
		return nil, apperror.InternalKey("aiimage.cos_secret_id.unavailable", nil, "COS SecretID 不可用")
	}
	secretKey, err := s.secretbox.Decrypt(cfg.SecretKeyEnc)
	if err != nil || strings.TrimSpace(secretKey) == "" {
		return nil, apperror.InternalKey("aiimage.cos_secret_key.unavailable", nil, "COS SecretKey 不可用")
	}
	return &cosRuntimeConfig{SecretID: secretID, SecretKey: secretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, BucketDomain: cfg.BucketDomain}, nil
}

type cosRuntimeConfig struct {
	SecretID     string
	SecretKey    string
	Bucket       string
	Region       string
	Endpoint     string
	BucketDomain string
}

func (s *Service) readCOSFile(ctx context.Context, cfg *cosRuntimeConfig, file ImageFile) (infraai.ImageAsset, *apperror.Error) {
	if s == nil || s.objectReader == nil {
		return infraai.ImageAsset{}, apperror.InternalKey("aiimage.cos_reader.missing", nil, "COS读取器未配置")
	}
	if cfg == nil {
		return infraai.ImageAsset{}, apperror.InternalKey("aiimage.cos_config.not_loaded", nil, "COS配置未加载")
	}
	if file.StorageProvider != StorageProviderCOS || strings.TrimSpace(file.StorageKey) == "" {
		return infraai.ImageAsset{}, apperror.BadRequestKey("aiimage.input_asset.cos_required", nil, "参考图必须来自已上传的 COS 图片文件")
	}
	result, err := s.objectReader.Get(ctx, storagecos.GetInput{SecretID: cfg.SecretID, SecretKey: cfg.SecretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, Key: file.StorageKey})
	if err != nil {
		return infraai.ImageAsset{}, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.asset.read_failed", nil, "读取图片文件失败", err)
	}
	mimeType := file.MimeType
	if strings.TrimSpace(result.ContentType) != "" {
		mimeType = strings.TrimSpace(result.ContentType)
	}
	return infraai.ImageAsset{Name: path.Base(file.StorageKey), MimeType: mimeType, Data: result.Body}, nil
}

func (s *Service) storeUploadedFiles(ctx context.Context, userID uint64, uploads []UploadedFileInput) ([]ImageFileInput, *apperror.Error) {
	if userID == 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if s == nil || s.objectWriter == nil {
		return nil, apperror.InternalKey("aiimage.cos_writer.missing", nil, "COS写入器未配置")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	cfg, appErr := s.loadCOSConfig(ctx, repo)
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	files := make([]ImageFileInput, 0, len(uploads))
	for index, upload := range uploads {
		body := upload.Body
		if len(body) == 0 {
			return nil, apperror.BadRequestKey("aiimage.upload.empty", nil, "上传图片不能为空")
		}
		mimeType := strings.TrimSpace(upload.MimeType)
		if mimeType == "" {
			mimeType = http.DetectContentType(body)
		}
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			return nil, apperror.BadRequestKey("aiimage.upload.mime.invalid", nil, "上传图片MIME类型不合法")
		}
		key, err := s.inputKey(userID, index, mimeType, now)
		if err != nil {
			return nil, apperror.InternalKey("aiimage.input_key.build_failed", nil, "参考图存储路径失败")
		}
		if err := s.objectWriter.Put(ctx, storagecos.PutInput{SecretID: cfg.SecretID, SecretKey: cfg.SecretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, Key: key, Body: body, ContentType: mimeType}); err != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.input.upload_failed", nil, "上传参考图失败", err)
		}
		files = append(files, ImageFileInput{StorageProvider: StorageProviderCOS, StorageKey: key, StorageURL: publicCOSURL(*cfg, key), MimeType: mimeType, SizeBytes: int64(len(body))})
	}
	return files, nil
}

func (s *Service) decryptProviderKey(apiKeyEnc string) (string, *apperror.Error) {
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return "", apperror.InternalKey("aiimage.provider.api_key_decrypt_failed", nil, "解密AI供应商API Key失败")
	}
	if strings.TrimSpace(apiKey) == "" {
		return "", apperror.BadRequestKey("aiimage.provider.api_key_missing", nil, "AI供应商API Key未配置")
	}
	return apiKey, nil
}
func (s *Service) imageEngine(config ImageEngineConfig) infraai.ImageEngine {
	if s == nil || s.engineFactory == nil {
		return nil
	}
	return s.engineFactory.NewImageEngine(config)
}

func (s *Service) persistOutputs(ctx context.Context, repo Repository, task ImageTask, result *infraai.ImageResult) *apperror.Error {
	now := s.now()
	files := make([]ImageFile, 0, len(result.Images))
	var cfg *cosRuntimeConfig
	for index, image := range result.Images {
		file, appErr := s.outputFile(ctx, repo, task, image, index, now, &cfg)
		if appErr != nil {
			return appErr
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return apperror.InternalKey("aiimage.generate.empty_result", nil, "图片生成结果为空")
	}
	if err := repo.AppendTaskFiles(ctx, files); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.output_relation.save_failed", nil, "保存生成图片文件失败", err)
	}
	return nil
}
func (s *Service) outputFile(ctx context.Context, repo Repository, task ImageTask, image infraai.GeneratedImage, index int, now time.Time, cfgRef **cosRuntimeConfig) (ImageFile, *apperror.Error) {
	mimeType := image.MimeType
	if strings.TrimSpace(mimeType) == "" {
		mimeType = mimeFromFormat(task.OutputFormat)
	}
	revisedPrompt := optionalString(image.RevisedPrompt)
	if strings.TrimSpace(image.B64JSON) == "" {
		urlValue := strings.TrimSpace(image.URL)
		if urlValue == "" || !validURL(urlValue) {
			return ImageFile{}, apperror.InternalKey("aiimage.output.url.invalid", nil, "生成图片URL不合法")
		}
		return ImageFile{TaskID: task.ID, Role: FileRoleOutput, SortOrder: index + 1, StorageProvider: StorageProviderRemoteURL, StorageURL: urlValue, MimeType: mimeType, RevisedPrompt: revisedPrompt, CreatedAt: now}, nil
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(image.B64JSON))
	if err != nil || len(body) == 0 {
		return ImageFile{}, apperror.InternalKey("aiimage.output.base64_decode_failed", nil, "生成图片base64解码失败")
	}
	width, height, appErr := decodeOutputImageDimensions(body)
	if appErr != nil {
		return ImageFile{}, appErr
	}
	if s == nil || s.objectWriter == nil {
		return ImageFile{}, apperror.InternalKey("aiimage.cos_writer.missing", nil, "COS写入器未配置")
	}
	if *cfgRef == nil {
		cfg, appErr := s.loadCOSConfig(ctx, repo)
		if appErr != nil {
			return ImageFile{}, appErr
		}
		*cfgRef = cfg
	}
	key, err := s.outputKey(task.ID, index, mimeType, now)
	if err != nil {
		return ImageFile{}, apperror.InternalKey("aiimage.output_key.build_failed", nil, "生成图片存储路径失败")
	}
	cfg := *cfgRef
	if err := s.objectWriter.Put(ctx, storagecos.PutInput{SecretID: cfg.SecretID, SecretKey: cfg.SecretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, Key: key, Body: body, ContentType: mimeType}); err != nil {
		return ImageFile{}, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.output.upload_failed", nil, "上传生成图片失败", err)
	}
	return ImageFile{TaskID: task.ID, Role: FileRoleOutput, SortOrder: index + 1, StorageProvider: StorageProviderCOS, StorageKey: key, StorageURL: publicCOSURL(*cfg, key), MimeType: mimeType, Width: width, Height: height, SizeBytes: int64(len(body)), RevisedPrompt: revisedPrompt, CreatedAt: now}, nil
}

func decodeOutputImageDimensions(body []byte) (int, int, *apperror.Error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, apperror.InternalKey("aiimage.output.dimension_decode_failed", nil, "生成图片尺寸解析失败")
	}
	return cfg.Width, cfg.Height, nil
}

func (s *Service) outputKey(taskID uint64, index int, mimeType string, now time.Time) (string, error) {
	randBytes := make([]byte, 6)
	if _, err := s.random(randBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("ai-images/%04d/%02d/%02d/%d-%02d-%s%s", now.Year(), int(now.Month()), now.Day(), taskID, index+1, hex.EncodeToString(randBytes), extensionForMime(mimeType)), nil
}

func (s *Service) inputKey(userID uint64, index int, mimeType string, now time.Time) (string, error) {
	randBytes := make([]byte, 6)
	if _, err := s.random(randBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("ai-image-inputs/%04d/%02d/%02d/%d-%02d-%s%s", now.Year(), int(now.Month()), now.Day(), userID, index+1, hex.EncodeToString(randBytes), extensionForMime(mimeType)), nil
}

func (s *Service) finishGenerateFailed(ctx context.Context, repo Repository, input GenerateInput, startedAt time.Time, message string, cause error) (*GenerateResult, error) {
	if err := s.finishFailed(ctx, repo, input, startedAt, message, cause); err != nil {
		return nil, err
	}
	return &GenerateResult{TaskID: input.TaskID, Status: StatusFailed}, nil
}
func (s *Service) finishGenerateFailedWithRun(ctx context.Context, repo Repository, input GenerateInput, runID int64, startedAt time.Time, message string, cause error) (*GenerateResult, error) {
	finishedAt := s.now()
	duration := uint(elapsedMS(startedAt, finishedAt))
	failInput := airun.FailInput{RunID: runID, Message: trimErrorMessage(message, cause), FinishedAt: finishedAt, DurationMS: duration}
	switch statusFromImageError(ctx, cause) {
	case enum.AIRunStatusCanceled:
		_ = s.runRecorder.Cancel(context.Background(), airun.CancelInput(failInput))
	case enum.AIRunStatusTimeout:
		_ = s.runRecorder.Timeout(context.Background(), airun.TimeoutInput(failInput))
	default:
		_ = s.runRecorder.Fail(context.Background(), failInput)
	}
	if err := repo.FinishTaskFailed(ctx, input.UserID, input.TaskID, failInput.Message, int(duration), finishedAt); err != nil {
		return nil, fmt.Errorf("finish ai image task failed state: %w", err)
	}
	return &GenerateResult{TaskID: input.TaskID, Status: StatusFailed}, nil
}
func (s *Service) finishFailed(ctx context.Context, repo Repository, input GenerateInput, startedAt time.Time, message string, cause error) error {
	message = trimErrorMessage(message, cause)
	finishedAt := s.now()
	if err := repo.FinishTaskFailed(ctx, input.UserID, input.TaskID, message, elapsedMS(startedAt, finishedAt), finishedAt); err != nil {
		return fmt.Errorf("finish ai image task failed state: %w", err)
	}
	return nil
}
func validateImageRunUsageStatus(result *infraai.ImageResult) *apperror.Error {
	if result == nil {
		return apperror.InternalKey("aiimage.run.usage_status_missing", nil, "AI图片供应商用量状态缺失")
	}
	switch result.UsageStatus {
	case infraai.UsageStatusReported, infraai.UsageStatusUnavailable:
		return nil
	default:
		return apperror.InternalKey("aiimage.run.usage_status_missing", nil, "AI图片供应商用量状态缺失")
	}
}
func statusFromImageError(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return enum.AIRunStatusCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return enum.AIRunStatusTimeout
	}
	return enum.AIRunStatusFailed
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Status = strings.TrimSpace(query.Status)
	return query
}
func taskDTO(row ImageTask) TaskDTO {
	return TaskDTO{ID: row.ID, Platform: row.Platform, AgentID: row.AgentID, AgentNameSnapshot: row.AgentNameSnapshot, ProviderIDSnapshot: row.ProviderIDSnapshot, ProviderNameSnapshot: row.ProviderNameSnapshot, ModelIDSnapshot: row.ModelIDSnapshot, ModelDisplayNameSnapshot: row.ModelDisplayNameSnapshot, Prompt: row.Prompt, Size: row.Size, Quality: row.Quality, OutputFormat: row.OutputFormat, OutputCompression: row.OutputCompression, Moderation: row.Moderation, N: row.N, Status: row.Status, StatusName: statusLabels[row.Status], ErrorMessage: row.ErrorMessage, ActualParamsJSON: rawJSONString(row.ActualParamsJSON), FinishedAt: formatOptionalTime(row.FinishedAt), ElapsedMS: row.ElapsedMS, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}
func fileDTO(row ImageFile) FileDTO {
	dto := FileDTO{ID: row.ID, TaskID: row.TaskID, Role: row.Role, SortOrder: row.SortOrder, StorageProvider: row.StorageProvider, StorageKey: row.StorageKey, StorageURL: row.StorageURL, MimeType: row.MimeType, Width: row.Width, Height: row.Height, SizeBytes: row.SizeBytes, RelatedFileID: row.RelatedFileID, CreatedAt: formatTime(row.CreatedAt)}
	if row.RevisedPrompt != nil {
		dto.RevisedPrompt = *row.RevisedPrompt
	}
	return dto
}
func detailResponse(task ImageTask, rows []ImageFile) *DetailResponse {
	response := &DetailResponse{Task: taskDTO(task), Inputs: []FileDTO{}, Outputs: []FileDTO{}}
	for _, row := range rows {
		file := fileDTO(row)
		switch row.Role {
		case FileRoleInput:
			response.Inputs = append(response.Inputs, file)
		case FileRoleMask:
			copyFile := file
			response.Mask = &copyFile
		case FileRoleOutput:
			response.Outputs = append(response.Outputs, file)
		}
	}
	return response
}
func stringOptions(values []string, labels map[string]string) []dict.Option[string] {
	options := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, dict.Option[string]{Label: labels[value], Value: value})
	}
	return options
}
func knownValue(value string, labels map[string]string) bool { _, ok := labels[value]; return ok }
func isStatus(value string) bool                             { _, ok := statusLabels[value]; return ok }
func sceneEnabled(raw string, expected string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == expected {
			return true
		}
	}
	return false
}
func validURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
func jsonString(value map[string]any) *string {
	if len(value) == 0 {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := string(data)
	return &out
}
func sanitizeRawResponse(raw []byte) *string {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	sanitizeJSON(payload)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	out := string(data)
	return &out
}
func sanitizeJSON(value map[string]any) {
	for key, item := range value {
		if strings.EqualFold(key, "b64_json") {
			value[key] = "[omitted]"
			continue
		}
		switch typed := item.(type) {
		case map[string]any:
			sanitizeJSON(typed)
		case []any:
			for _, child := range typed {
				if childMap, ok := child.(map[string]any); ok {
					sanitizeJSON(childMap)
				}
			}
		}
	}
}
func rawJSONString(raw *string) json.RawMessage {
	if raw == nil || strings.TrimSpace(*raw) == "" || !json.Valid([]byte(*raw)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(*raw)
}
func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
func trimErrorMessage(message string, cause error) string {
	message = strings.TrimSpace(message)
	if message == "" && cause != nil {
		message = cause.Error()
	}
	if cause != nil && message != cause.Error() {
		message = message + ": " + cause.Error()
	}
	if len([]rune(message)) > 1000 {
		return string([]rune(message)[:1000])
	}
	if message == "" {
		return "图片生成失败"
	}
	return message
}
func elapsedMS(startedAt time.Time, finishedAt time.Time) int {
	if finishedAt.Before(startedAt) {
		return 0
	}
	return int(finishedAt.Sub(startedAt).Milliseconds())
}
func totalPage(total int64, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}
func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(serviceTimeLayout)
}
func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}
func mimeFromFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}
func extensionForMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
func publicCOSURL(cfg cosRuntimeConfig, key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if strings.TrimSpace(cfg.BucketDomain) != "" {
		return publicURLJoin(cfg.BucketDomain, key)
	}
	if strings.TrimSpace(cfg.Endpoint) != "" {
		return publicURLJoin(cfg.Endpoint, key)
	}
	return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", cfg.Bucket, cfg.Region, key)
}
func publicURLJoin(base string, key string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return strings.TrimLeft(strings.TrimSpace(key), "/")
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + strings.TrimLeft(base, "/")
	}
	return base + "/" + strings.TrimLeft(strings.TrimSpace(key), "/")
}
