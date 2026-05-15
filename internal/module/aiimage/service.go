package aiimage

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
	storagecos "admin_back_go/internal/platform/storage/cos"
	"admin_back_go/internal/platform/taskqueue"

	"gorm.io/gorm"
)

const (
	SceneImageGenerate = "image_generate"
	RequiredModelID    = "gpt-image-2"

	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"

	AssetRoleInput  = "input"
	AssetRoleMask   = "mask"
	AssetRoleOutput = "output"

	SourceTypeUpload    = "upload"
	SourceTypeMask      = "mask"
	SourceTypeGenerated = "generated"

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
	sizeLabels       = map[string]string{"auto": "自动", "1024x1024": "1024×1024", "1536x1024": "1536×1024", "1024x1536": "1024×1536"}
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
	NewImageEngine(config ImageEngineConfig) platformai.ImageEngine
}

type ImageEngineFactoryFunc func(config ImageEngineConfig) platformai.ImageEngine

func (f ImageEngineFactoryFunc) NewImageEngine(config ImageEngineConfig) platformai.ImageEngine {
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
	return &Service{repository: deps.Repository, enqueuer: deps.Enqueuer, secretbox: deps.Secretbox, engineFactory: deps.EngineFactory, objectReader: deps.ObjectReader, objectWriter: deps.ObjectWriter, now: deps.Now, random: deps.Random}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agents, err := repo.ListImageAgents(ctx)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.agent.query_failed", nil, "查询图片智能体失败", err)
	}
	if agents == nil {
		agents = []AgentOption{}
	}
	return &PageInitResponse{Dict: PageInitDict{SizeArr: stringOptions([]string{"auto", "1024x1024", "1536x1024", "1024x1536"}, sizeLabels), QualityArr: stringOptions([]string{"auto", "low", "medium", "high"}, qualityLabels), OutputFormatArr: stringOptions([]string{"png", "jpeg", "webp"}, formatLabels), ModerationArr: stringOptions([]string{"auto", "low"}, moderationLabels), StatusArr: stringOptions([]string{StatusPending, StatusRunning, StatusSuccess, StatusFailed}, statusLabels), FavoriteArr: dict.CommonYesNoOptions()}, AgentOptions: agents}, nil
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

func (s *Service) Detail(ctx context.Context, userID uint64, taskID uint64) (*DetailResponse, *apperror.Error) {
	if userID == 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if taskID == 0 {
		return nil, apperror.BadRequestKey("aiimage.task.id.invalid", nil, "无效的图片任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	task, err := repo.GetTask(ctx, userID, taskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task.query_failed", nil, "查询图片任务失败", err)
	}
	if task == nil {
		return nil, apperror.NotFoundKey("aiimage.task.not_found", nil, "图片任务不存在")
	}
	assets, err := repo.LoadTaskAssets(ctx, taskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task_assets.query_failed", nil, "查询图片任务资产失败", err)
	}
	return detailResponse(*task, assets), nil
}

func (s *Service) RegisterAsset(ctx context.Context, input RegisterAssetInput) (*AssetDTO, *apperror.Error) {
	row, appErr := normalizeAssetInput(input)
	if appErr != nil {
		return nil, appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	row.CreatedAt = now
	row.UpdatedAt = now
	id, err := repo.CreateAsset(ctx, row)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.asset.register_failed", nil, "注册图片资产失败", err)
	}
	row.ID = id
	dto := assetDTO(row)
	return &dto, nil
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
	agent, appErr := s.validImageAgent(ctx, repo, normalized.AgentID)
	if appErr != nil {
		return nil, appErr
	}
	assets, appErr := s.validInputAssets(ctx, repo, normalized)
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	task := ImageTask{UserID: normalized.UserID, AgentID: normalized.AgentID, AgentNameSnapshot: agent.AgentName, ProviderIDSnapshot: agent.ProviderID, ProviderNameSnapshot: agent.ProviderName, ModelIDSnapshot: agent.ModelID, ModelDisplayNameSnapshot: agent.ModelDisplayName, Prompt: normalized.Prompt, Size: normalized.Size, Quality: normalized.Quality, OutputFormat: normalized.OutputFormat, OutputCompression: normalized.OutputCompression, Moderation: normalized.Moderation, N: normalized.N, Status: StatusPending, IsFavorite: enum.CommonNo, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now}
	links := inputLinks(normalized, assets, now)
	id, err := repo.CreateTaskWithAssets(ctx, task, links)
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

func (s *Service) Favorite(ctx context.Context, input FavoriteInput) (*TaskDTO, *apperror.Error) {
	if input.UserID == 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.TaskID == 0 {
		return nil, apperror.BadRequestKey("aiimage.task.id.invalid", nil, "无效的图片任务ID")
	}
	if !enum.IsCommonYesNo(input.IsFavorite) {
		return nil, apperror.BadRequestKey("aiimage.favorite.status.invalid", nil, "无效的收藏状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if err := repo.UpdateFavorite(ctx, input.UserID, input.TaskID, input.IsFavorite); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFoundKey("aiimage.task.not_found", nil, "图片任务不存在")
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.favorite.update_failed", nil, "更新图片收藏失败", err)
	}
	task, err := repo.GetTask(ctx, input.UserID, input.TaskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.task.query_failed", nil, "查询图片任务失败", err)
	}
	if task == nil {
		return nil, apperror.NotFoundKey("aiimage.task.not_found", nil, "图片任务不存在")
	}
	dto := taskDTO(*task)
	return &dto, nil
}

func (s *Service) Delete(ctx context.Context, userID uint64, taskID uint64) *apperror.Error {
	if userID == 0 {
		return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if taskID == 0 {
		return apperror.BadRequestKey("aiimage.task.id.invalid", nil, "无效的图片任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.SoftDeleteTask(ctx, userID, taskID); err != nil {
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
		return nil, fmt.Errorf("claim image task: %w", err)
	}
	if !claimed {
		task, loadErr := repo.GetTaskForWorker(ctx, input.UserID, input.TaskID)
		if loadErr != nil {
			return nil, fmt.Errorf("load unclaimed image task: %w", loadErr)
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
	agent, appErr := s.validImageAgent(ctx, repo, task.AgentID)
	if appErr != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, appErr.Message, appErr)
	}
	links, err := repo.LoadTaskAssets(ctx, task.ID)
	if err != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "读取图片任务资产失败", err)
	}
	assets, appErr := s.engineAssets(ctx, repo, links)
	if appErr != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, appErr.Message, appErr)
	}
	apiKey, appErr := s.decryptProviderKey(agent.APIKeyEnc)
	if appErr != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, appErr.Message, appErr)
	}
	engine := s.imageEngine(ImageEngineConfig{EngineType: agent.EngineType, BaseURL: agent.BaseURL, APIKey: apiKey, Timeout: 10 * time.Minute})
	if engine == nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "图片生成引擎未配置", nil)
	}
	result, err := engine.GenerateImages(ctx, platformai.ImageInput{Model: task.ModelIDSnapshot, Prompt: task.Prompt, Size: task.Size, Quality: task.Quality, OutputFormat: task.OutputFormat, OutputCompression: task.OutputCompression, Moderation: task.Moderation, N: task.N, InputAssets: assets.inputs, MaskAsset: assets.mask})
	if err != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "图片生成失败", err)
	}
	if result == nil || len(result.Images) == 0 {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, "图片生成结果为空", nil)
	}
	if appErr := s.persistOutputs(ctx, repo, *task, result); appErr != nil {
		return s.finishGenerateFailed(context.Background(), repo, input, startedAt, appErr.Message, appErr)
	}
	finishedAt := s.now()
	actualParamsJSON := jsonString(result.ActualParams)
	rawResponseJSON := sanitizeRawResponse(result.RawResponse)
	if err := repo.FinishTaskSuccess(ctx, task.UserID, task.ID, actualParamsJSON, rawResponseJSON, elapsedMS(startedAt, finishedAt), finishedAt); err != nil {
		return nil, fmt.Errorf("finish image task success: %w", err)
	}
	return &GenerateResult{TaskID: task.ID, Status: StatusSuccess}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("aiimage.repository_missing", nil, "AI图片仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) normalizeCreateInput(input CreateInput) (CreateInput, *apperror.Error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Size = strings.TrimSpace(input.Size)
	input.Quality = strings.TrimSpace(input.Quality)
	input.OutputFormat = strings.ToLower(strings.TrimSpace(input.OutputFormat))
	input.Moderation = strings.TrimSpace(input.Moderation)
	input.InputAssetIDs = uniqueIDs(input.InputAssetIDs)
	if input.UserID == 0 {
		return CreateInput{}, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.AgentID == 0 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.agent.required", nil, "图片智能体不能为空")
	}
	if input.Prompt == "" {
		return CreateInput{}, apperror.BadRequestKey("aiimage.prompt.required", nil, "提示词不能为空")
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
	if input.N > 4 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.n.too_many", nil, "单次最多生成4张图片")
	}
	if len(input.InputAssetIDs) > 10 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.input_assets.too_many", nil, "参考图最多10张")
	}
	if input.MaskAssetID > 0 && len(input.InputAssetIDs) == 0 {
		return CreateInput{}, apperror.BadRequestKey("aiimage.mask.requires_input", nil, "遮罩图必须配合参考图使用")
	}
	if input.MaskTargetAssetID > 0 && !containsID(input.InputAssetIDs, input.MaskTargetAssetID) {
		return CreateInput{}, apperror.BadRequestKey("aiimage.mask_target.invalid", nil, "遮罩目标图必须在参考图中")
	}
	return input, nil
}

func normalizeAssetInput(input RegisterAssetInput) (ImageAsset, *apperror.Error) {
	provider := strings.TrimSpace(input.StorageProvider)
	key := strings.TrimLeft(strings.TrimSpace(input.StorageKey), "/")
	urlValue := strings.TrimSpace(input.StorageURL)
	mimeType := strings.TrimSpace(input.MimeType)
	sourceType := strings.TrimSpace(input.SourceType)
	if input.UserID == 0 {
		return ImageAsset{}, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if provider != StorageProviderCOS && provider != StorageProviderRemoteURL {
		return ImageAsset{}, apperror.BadRequestKey("aiimage.asset.storage_provider.unsupported", nil, "不支持的图片存储类型")
	}
	if provider == StorageProviderCOS && key == "" {
		return ImageAsset{}, apperror.BadRequestKey("aiimage.asset.cos_key.required", nil, "COS图片key不能为空")
	}
	if urlValue == "" {
		return ImageAsset{}, apperror.BadRequestKey("aiimage.asset.url.required", nil, "图片URL不能为空")
	}
	if !validURL(urlValue) {
		return ImageAsset{}, apperror.BadRequestKey("aiimage.asset.url.invalid", nil, "图片URL不合法")
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return ImageAsset{}, apperror.BadRequestKey("aiimage.asset.mime.invalid", nil, "图片MIME类型不合法")
	}
	if sourceType != SourceTypeUpload && sourceType != SourceTypeMask {
		return ImageAsset{}, apperror.BadRequestKey("aiimage.asset.source_type.unsupported", nil, "图片资产来源不支持")
	}
	return ImageAsset{UserID: input.UserID, StorageProvider: provider, StorageKey: key, StorageURL: urlValue, MimeType: mimeType, Width: input.Width, Height: input.Height, SizeBytes: input.SizeBytes, SourceType: sourceType, IsDel: enum.CommonNo}, nil
}

func (s *Service) validImageAgent(ctx context.Context, repo Repository, agentID uint64) (*AgentRuntime, *apperror.Error) {
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
	if !sceneEnabled(agent.ScenesJSON, SceneImageGenerate) {
		return nil, apperror.BadRequestKey("aiimage.agent.scene_missing", nil, "智能体未启用图片生成场景")
	}
	if strings.TrimSpace(agent.ModelID) != RequiredModelID {
		return nil, apperror.BadRequestKey("aiimage.model.unsupported", nil, "图片工作台首版只支持 gpt-image-2")
	}
	if strings.TrimSpace(agent.APIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("aiimage.provider.api_key_missing", nil, "AI供应商API Key未配置")
	}
	if platformai.EngineType(agent.EngineType) != platformai.EngineTypeOpenAI {
		return nil, apperror.BadRequestKey("aiimage.provider.unsupported", nil, "图片工作台只支持 OpenAI-compatible 供应商")
	}
	return agent, nil
}

func (s *Service) validInputAssets(ctx context.Context, repo Repository, input CreateInput) (map[uint64]ImageAsset, *apperror.Error) {
	ids := append([]uint64{}, input.InputAssetIDs...)
	if input.MaskAssetID > 0 {
		ids = append(ids, input.MaskAssetID)
	}
	assets, err := repo.LoadAssetsByIDs(ctx, input.UserID, ids)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.asset.query_failed", nil, "查询图片资产失败", err)
	}
	if len(assets) != len(uniqueIDs(ids)) {
		return nil, apperror.BadRequestKey("aiimage.asset.not_owned", nil, "图片资产不存在或不属于当前用户")
	}
	byID := make(map[uint64]ImageAsset, len(assets))
	for _, asset := range assets {
		if asset.StorageProvider != StorageProviderCOS || strings.TrimSpace(asset.StorageKey) == "" {
			return nil, apperror.BadRequestKey("aiimage.input_asset.cos_required", nil, "参考图必须来自已上传的 COS 图片资产")
		}
		if asset.SourceType != SourceTypeUpload && asset.SourceType != SourceTypeMask && asset.SourceType != SourceTypeGenerated {
			return nil, apperror.BadRequestKey("aiimage.asset.source_type.unsupported", nil, "图片资产来源不支持")
		}
		byID[asset.ID] = asset
	}
	return byID, nil
}

func inputLinks(input CreateInput, assets map[uint64]ImageAsset, now time.Time) []ImageTaskAsset {
	links := make([]ImageTaskAsset, 0, len(input.InputAssetIDs)+1)
	for index, id := range input.InputAssetIDs {
		if _, ok := assets[id]; ok {
			links = append(links, ImageTaskAsset{AssetID: id, Role: AssetRoleInput, SortOrder: index + 1, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now})
		}
	}
	if input.MaskAssetID > 0 {
		var related *uint64
		if input.MaskTargetAssetID > 0 {
			value := input.MaskTargetAssetID
			related = &value
		}
		links = append(links, ImageTaskAsset{AssetID: input.MaskAssetID, Role: AssetRoleMask, SortOrder: 1, RelatedAssetID: related, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now})
	}
	return links
}

type preparedEngineAssets struct {
	inputs []platformai.ImageAsset
	mask   *platformai.ImageAsset
}

func (s *Service) engineAssets(ctx context.Context, repo Repository, rows []TaskAssetRow) (*preparedEngineAssets, *apperror.Error) {
	var inputRows []TaskAssetRow
	var maskRow *TaskAssetRow
	for _, row := range rows {
		switch row.Role {
		case AssetRoleInput:
			inputRows = append(inputRows, row)
		case AssetRoleMask:
			copyRow := row
			maskRow = &copyRow
		}
	}
	if len(inputRows) == 0 && maskRow == nil {
		return &preparedEngineAssets{}, nil
	}
	cfg, appErr := s.loadCOSConfig(ctx, repo)
	if appErr != nil {
		return nil, appErr
	}
	out := &preparedEngineAssets{inputs: make([]platformai.ImageAsset, 0, len(inputRows))}
	for _, row := range inputRows {
		asset, appErr := s.readCOSAsset(ctx, cfg, row.Asset)
		if appErr != nil {
			return nil, appErr
		}
		out.inputs = append(out.inputs, asset)
	}
	if maskRow != nil {
		asset, appErr := s.readCOSAsset(ctx, cfg, maskRow.Asset)
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

func (s *Service) readCOSAsset(ctx context.Context, cfg *cosRuntimeConfig, asset ImageAsset) (platformai.ImageAsset, *apperror.Error) {
	if s == nil || s.objectReader == nil {
		return platformai.ImageAsset{}, apperror.InternalKey("aiimage.cos_reader.missing", nil, "COS读取器未配置")
	}
	if cfg == nil {
		return platformai.ImageAsset{}, apperror.InternalKey("aiimage.cos_config.not_loaded", nil, "COS配置未加载")
	}
	result, err := s.objectReader.Get(ctx, storagecos.GetInput{SecretID: cfg.SecretID, SecretKey: cfg.SecretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, Key: asset.StorageKey})
	if err != nil {
		return platformai.ImageAsset{}, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.asset.read_failed", nil, "读取图片资产失败", err)
	}
	mimeType := asset.MimeType
	if strings.TrimSpace(result.ContentType) != "" {
		mimeType = strings.TrimSpace(result.ContentType)
	}
	return platformai.ImageAsset{Name: path.Base(asset.StorageKey), MimeType: mimeType, Data: result.Body}, nil
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

func (s *Service) imageEngine(config ImageEngineConfig) platformai.ImageEngine {
	if s == nil || s.engineFactory == nil {
		return nil
	}
	return s.engineFactory.NewImageEngine(config)
}

func (s *Service) persistOutputs(ctx context.Context, repo Repository, task ImageTask, result *platformai.ImageResult) *apperror.Error {
	now := s.now()
	links := make([]ImageTaskAsset, 0, len(result.Images))
	var cfg *cosRuntimeConfig
	for index, image := range result.Images {
		asset, appErr := s.outputAsset(ctx, repo, task, image, index, now, &cfg)
		if appErr != nil {
			return appErr
		}
		assetID, err := repo.CreateAsset(ctx, asset)
		if err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.output_asset.save_failed", nil, "保存生成图片资产失败", err)
		}
		actualParams := jsonString(result.ActualParams)
		revisedPrompt := optionalString(image.RevisedPrompt)
		links = append(links, ImageTaskAsset{TaskID: task.ID, AssetID: assetID, Role: AssetRoleOutput, SortOrder: index + 1, ActualParamsJSON: actualParams, RevisedPrompt: revisedPrompt, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now})
	}
	if len(links) == 0 {
		return apperror.InternalKey("aiimage.generate.empty_result", nil, "图片生成结果为空")
	}
	if err := repo.AppendTaskAssets(ctx, links); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.output_relation.save_failed", nil, "保存生成图片关系失败", err)
	}
	return nil
}

func (s *Service) outputAsset(ctx context.Context, repo Repository, task ImageTask, image platformai.GeneratedImage, index int, now time.Time, cfgRef **cosRuntimeConfig) (ImageAsset, *apperror.Error) {
	mimeType := image.MimeType
	if strings.TrimSpace(mimeType) == "" {
		mimeType = mimeFromFormat(task.OutputFormat)
	}
	if strings.TrimSpace(image.B64JSON) == "" {
		urlValue := strings.TrimSpace(image.URL)
		if urlValue == "" || !validURL(urlValue) {
			return ImageAsset{}, apperror.InternalKey("aiimage.output.url.invalid", nil, "生成图片URL不合法")
		}
		return ImageAsset{UserID: task.UserID, StorageProvider: StorageProviderRemoteURL, StorageURL: urlValue, MimeType: mimeType, SourceType: SourceTypeGenerated, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now}, nil
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(image.B64JSON))
	if err != nil || len(body) == 0 {
		return ImageAsset{}, apperror.InternalKey("aiimage.output.base64_decode_failed", nil, "生成图片base64解码失败")
	}
	if s == nil || s.objectWriter == nil {
		return ImageAsset{}, apperror.InternalKey("aiimage.cos_writer.missing", nil, "COS写入器未配置")
	}
	if *cfgRef == nil {
		cfg, appErr := s.loadCOSConfig(ctx, repo)
		if appErr != nil {
			return ImageAsset{}, appErr
		}
		*cfgRef = cfg
	}
	key, err := s.outputKey(task.ID, index, mimeType, now)
	if err != nil {
		return ImageAsset{}, apperror.InternalKey("aiimage.output_key.build_failed", nil, "生成图片存储路径失败")
	}
	cfg := *cfgRef
	if err := s.objectWriter.Put(ctx, storagecos.PutInput{SecretID: cfg.SecretID, SecretKey: cfg.SecretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, Key: key, Body: body, ContentType: mimeType}); err != nil {
		return ImageAsset{}, apperror.WrapKey(apperror.CodeInternal, 500, "aiimage.output.upload_failed", nil, "上传生成图片失败", err)
	}
	return ImageAsset{UserID: task.UserID, StorageProvider: StorageProviderCOS, StorageKey: key, StorageURL: publicCOSURL(*cfg, key), MimeType: mimeType, SizeBytes: int64(len(body)), SourceType: SourceTypeGenerated, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) outputKey(taskID uint64, index int, mimeType string, now time.Time) (string, error) {
	randBytes := make([]byte, 6)
	if _, err := s.random(randBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("ai-images/%04d/%02d/%02d/%d-%02d-%s%s", now.Year(), int(now.Month()), now.Day(), taskID, index+1, hex.EncodeToString(randBytes), extensionForMime(mimeType)), nil
}

func (s *Service) finishGenerateFailed(ctx context.Context, repo Repository, input GenerateInput, startedAt time.Time, message string, cause error) (*GenerateResult, error) {
	if err := s.finishFailed(ctx, repo, input, startedAt, message, cause); err != nil {
		return nil, err
	}
	return &GenerateResult{TaskID: input.TaskID, Status: StatusFailed}, nil
}

func (s *Service) finishFailed(ctx context.Context, repo Repository, input GenerateInput, startedAt time.Time, message string, cause error) error {
	message = trimErrorMessage(message, cause)
	finishedAt := s.now()
	if err := repo.FinishTaskFailed(ctx, input.UserID, input.TaskID, message, elapsedMS(startedAt, finishedAt), finishedAt); err != nil {
		return fmt.Errorf("finish image task failed state: %w", err)
	}
	return nil
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
	return TaskDTO{ID: row.ID, AgentID: row.AgentID, AgentNameSnapshot: row.AgentNameSnapshot, ProviderIDSnapshot: row.ProviderIDSnapshot, ProviderNameSnapshot: row.ProviderNameSnapshot, ModelIDSnapshot: row.ModelIDSnapshot, ModelDisplayNameSnapshot: row.ModelDisplayNameSnapshot, Prompt: row.Prompt, Size: row.Size, Quality: row.Quality, OutputFormat: row.OutputFormat, OutputCompression: row.OutputCompression, Moderation: row.Moderation, N: row.N, Status: row.Status, StatusName: statusLabels[row.Status], ErrorMessage: row.ErrorMessage, ActualParamsJSON: rawJSONString(row.ActualParamsJSON), IsFavorite: row.IsFavorite, FinishedAt: formatOptionalTime(row.FinishedAt), ElapsedMS: row.ElapsedMS, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func assetDTO(row ImageAsset) AssetDTO {
	return AssetDTO{ID: row.ID, StorageProvider: row.StorageProvider, StorageKey: row.StorageKey, StorageURL: row.StorageURL, MimeType: row.MimeType, Width: row.Width, Height: row.Height, SizeBytes: row.SizeBytes, SourceType: row.SourceType, CreatedAt: formatTime(row.CreatedAt)}
}

func detailResponse(task ImageTask, rows []TaskAssetRow) *DetailResponse {
	response := &DetailResponse{Task: taskDTO(task), Inputs: []AssetDTO{}, Outputs: []AssetDTO{}}
	for _, row := range rows {
		asset := assetDTO(row.Asset)
		asset.Role = row.Role
		asset.SortOrder = row.SortOrder
		asset.RelatedAssetID = row.RelatedAssetID
		asset.ActualParamsJSON = rawJSONString(row.ActualParamsJSON)
		if row.RevisedPrompt != nil {
			asset.RevisedPrompt = *row.RevisedPrompt
		}
		switch row.Role {
		case AssetRoleInput:
			response.Inputs = append(response.Inputs, asset)
		case AssetRoleMask:
			copyAsset := asset
			response.Mask = &copyAsset
		case AssetRoleOutput:
			response.Outputs = append(response.Outputs, asset)
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

func knownValue(value string, labels map[string]string) bool {
	_, ok := labels[value]
	return ok
}

func isStatus(value string) bool {
	_, ok := statusLabels[value]
	return ok
}

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

func containsID(ids []uint64, expected uint64) bool {
	for _, id := range ids {
		if id == expected {
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
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return json.RawMessage("{}")
	}
	if !json.Valid([]byte(*raw)) {
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
