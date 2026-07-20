package aivideo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/capability"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const (
	maxReferenceMediaBytes = 100 << 20
)

type Service struct {
	repository    Repository
	secretbox     Secretbox
	engineFactory EngineFactory
	runRecorder   RunRecorder
	objectWriter  storagecos.ObjectWriter
	now           func() time.Time
	random        func([]byte) (int, error)
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	randomSource := deps.Random
	if randomSource == nil {
		randomSource = rand.Read
	}
	return &Service{repository: deps.Repository, secretbox: deps.Secretbox, engineFactory: deps.EngineFactory, runRecorder: deps.RunRecorder, objectWriter: deps.ObjectWriter, now: now, random: randomSource}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if !enum.IsRegisteredPlatform(input.Platform) {
		return nil, apperror.BadRequestKey("aivideo.platform.invalid", nil, "无效的视频生成平台")
	}
	if input.UserID <= 0 || input.AgentID <= 0 || input.Prompt == "" {
		return nil, apperror.BadRequestKey("aivideo.request.invalid", nil, "视频生成参数错误")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agent, appErr := s.validVideoAgent(ctx, repo, input.AgentID)
	if appErr != nil {
		return nil, appErr
	}
	engine, appErr := s.engine(ctx, *agent)
	if appErr != nil {
		return nil, appErr
	}
	modelID := strings.TrimSpace(agent.ModelID)
	now := s.now()
	localTask := VideoTask{
		Platform:        input.Platform,
		UserID:          input.UserID,
		AgentID:         input.AgentID,
		ProviderID:      agent.ProviderID,
		ModelID:         modelID,
		Prompt:          input.Prompt,
		DurationSeconds: input.DurationSeconds,
		Size:            input.Size,
		ResolutionName:  input.ResolutionName,
		Status:          StatusPending,
		IsDel:           IsDelActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	taskID, err := repo.CreateTask(ctx, localTask)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_create_failed", nil, "创建AI视频任务失败", err)
	}
	if s.runRecorder == nil {
		if markErr := s.markTaskFailed(input.Platform, input.UserID, taskID, "AI视频运行记录服务未配置"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", markErr)
		}
		return nil, apperror.InternalKey("aivideo.run_recorder_missing", nil, "AI视频运行记录服务未配置")
	}
	runStartedAt := now
	runID, err := s.runRecorder.Start(ctx, airun.StartInput{
		Platform:         input.Platform,
		RequestID:        videoRunRequestID(taskID),
		UserID:           input.UserID,
		AgentID:          input.AgentID,
		ProviderID:       agent.ProviderID,
		ModelID:          modelID,
		ModelDisplayName: agent.ModelDisplayName,
		InputSnapshot:    input.Prompt,
		StartedAt:        runStartedAt,
	})
	if err != nil {
		if markErr := s.markTaskFailed(input.Platform, input.UserID, taskID, "创建AI视频运行记录失败"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", markErr)
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.run_start_failed", nil, "创建AI视频运行记录失败", err)
	}
	if err := repo.UpdateTask(ctx, input.Platform, input.UserID, taskID, map[string]any{"run_id": runID, "updated_at": s.now()}); err != nil {
		s.failRun(context.Background(), runID, "绑定AI视频运行记录失败", runStartedAt)
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", err)
	}
	providerTask, err := engine.CreateVideo(ctx, infraai.VideoInput{Model: modelID, Prompt: input.Prompt, DurationSeconds: input.DurationSeconds, Size: input.Size, ResolutionName: input.ResolutionName, GenerateAudio: input.GenerateAudio, Watermark: input.Watermark})
	if err != nil {
		s.failRun(context.Background(), runID, "AI视频生成失败", runStartedAt)
		if markErr := s.markTaskFailed(input.Platform, input.UserID, taskID, "AI视频生成失败"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", markErr)
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.provider_failed", nil, "AI视频生成失败", err)
	}
	providerTaskID := ""
	if providerTask != nil {
		providerTaskID = strings.TrimSpace(providerTask.ID)
	}
	if providerTaskID == "" {
		s.failRun(context.Background(), runID, "AI视频任务创建结果无效", runStartedAt)
		if markErr := s.markTaskFailed(input.Platform, input.UserID, taskID, "AI视频任务创建结果无效"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", markErr)
		}
		return nil, apperror.InternalKey("aivideo.provider_task_invalid", nil, "AI视频任务创建结果无效")
	}
	status, ok := normalizeVideoStatus(providerTask.Status)
	if !ok {
		s.failRun(context.Background(), runID, "AI视频任务状态无效", runStartedAt)
		return nil, apperror.InternalKey("aivideo.provider_status_invalid", nil, "AI视频任务状态无效")
	}
	fields := map[string]any{"provider_task_id": providerTaskID, "provider_id": agent.ProviderID, "model_id": modelID, "status": status, "updated_at": s.now()}
	if isTerminalStatus(status) {
		fields["finished_at"] = s.now()
	}
	if err := repo.UpdateTask(ctx, input.Platform, input.UserID, taskID, fields); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", err)
	}
	if isTerminalStatus(status) {
		if appErr := s.finishRunForVideoStatus(context.Background(), runID, status, "", runStartedAt); appErr != nil {
			return nil, appErr
		}
	}
	return &CreateResponse{ID: taskID, Status: status}, nil
}

func (s *Service) Status(ctx context.Context, platform string, userID int64, id int64) (*StatusResponse, *apperror.Error) {
	task, appErr := s.ownedTask(ctx, platform, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	engine, providerTaskID, appErr := s.engineForTask(ctx, task)
	if appErr != nil {
		return nil, appErr
	}
	providerTask, err := engine.GetVideo(ctx, providerTaskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.provider_status_failed", nil, "查询AI视频任务失败", err)
	}
	if providerTask == nil {
		return nil, apperror.InternalKey("aivideo.provider_status_invalid", nil, "AI视频任务状态无效")
	}
	status, ok := normalizeVideoStatus(providerTask.Status)
	if !ok {
		return nil, apperror.InternalKey("aivideo.provider_status_invalid", nil, "AI视频任务状态无效")
	}
	fields := map[string]any{"status": status, "error_message": strings.TrimSpace(providerTask.ErrorMessage), "updated_at": s.now()}
	if isTerminalStatus(status) {
		fields["finished_at"] = s.now()
	}
	if err := s.repository.UpdateTask(ctx, task.Platform, userID, id, fields); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_update_failed", nil, "更新AI视频任务失败", err)
	}
	if isTerminalStatus(status) {
		if appErr := s.finishRunForVideoStatus(context.Background(), task.RunID, status, providerTask.ErrorMessage, task.CreatedAt); appErr != nil {
			return nil, appErr
		}
	}
	return &StatusResponse{ID: task.ID, Status: status}, nil
}

func (s *Service) Content(ctx context.Context, platform string, userID int64, id int64) ([]byte, string, *apperror.Error) {
	task, appErr := s.ownedTask(ctx, platform, userID, id)
	if appErr != nil {
		return nil, "", appErr
	}
	engine, providerTaskID, appErr := s.engineForTask(ctx, task)
	if appErr != nil {
		return nil, "", appErr
	}
	body, contentType, err := engine.DownloadVideo(ctx, providerTaskID)
	if err != nil {
		return nil, "", apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.provider_content_failed", nil, "下载AI视频内容失败", err)
	}
	if len(body) == 0 {
		return nil, "", apperror.BadRequestKey("aivideo.content_empty", nil, "AI视频内容为空")
	}
	return body, contentType, nil
}

func (s *Service) UploadReferenceMedia(ctx context.Context, input ReferenceMediaUploadInput) (*ReferenceMediaUploadResponse, *apperror.Error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.FileName = strings.TrimSpace(input.FileName)
	input.MediaKind = strings.ToLower(strings.TrimSpace(input.MediaKind))
	input.MimeType = referenceMimeType(input.MimeType, input.FileName, input.Body)
	if !enum.IsRegisteredPlatform(input.Platform) {
		return nil, apperror.BadRequestKey("aivideo.platform.invalid", nil, "无效的视频生成平台")
	}
	if input.UserID <= 0 {
		return nil, apperror.BadRequestKey("aivideo.reference_media.request.invalid", nil, "参考媒体上传参数错误")
	}
	if !isReferenceMediaKind(input.MediaKind) {
		return nil, apperror.BadRequestKey("aivideo.reference_media.kind.invalid", nil, "参考媒体类型无效")
	}
	if len(input.Body) == 0 {
		return nil, apperror.BadRequestKey("aivideo.reference_media.empty", nil, "参考媒体不能为空")
	}
	if len(input.Body) > maxReferenceMediaBytes {
		return nil, apperror.BadRequestKey("aivideo.reference_media.too_large", nil, "参考媒体文件过大")
	}
	if !referenceMimeMatchesKind(input.MediaKind, input.MimeType) {
		return nil, apperror.BadRequestKey("aivideo.reference_media.mime.invalid", nil, "参考媒体MIME类型不合法")
	}
	if s == nil || s.objectWriter == nil {
		return nil, apperror.InternalKey("aivideo.reference_media.cos_writer_missing", nil, "参考媒体存储写入器未配置")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	cfg, appErr := s.loadCOSConfig(ctx, repo)
	if appErr != nil {
		return nil, appErr
	}
	randomID, err := s.referenceRandomID()
	if err != nil {
		return nil, apperror.InternalKey("aivideo.reference_media.key_failed", nil, "参考媒体存储路径失败")
	}
	key := referenceMediaKey(input.Platform, input.UserID, input.MediaKind, input.MimeType, s.now(), randomID)
	if err := s.objectWriter.Put(ctx, storagecos.PutInput{SecretID: cfg.SecretID, SecretKey: cfg.SecretKey, Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, Key: key, Body: input.Body, ContentType: input.MimeType}); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.reference_media.upload_failed", nil, "上传参考媒体失败", err)
	}
	return &ReferenceMediaUploadResponse{
		ID:              "ref_" + randomID,
		URL:             publicCOSURL(*cfg, key),
		StorageProvider: StorageProviderCOS,
		StorageKey:      key,
		MimeType:        input.MimeType,
		MediaKind:       input.MediaKind,
		Bytes:           int64(len(input.Body)),
	}, nil
}

func (s *Service) ownedTask(ctx context.Context, platform string, userID int64, id int64) (*VideoTask, *apperror.Error) {
	platform = strings.TrimSpace(platform)
	if !enum.IsRegisteredPlatform(platform) {
		return nil, apperror.BadRequestKey("aivideo.platform.invalid", nil, "无效的视频生成平台")
	}
	if userID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if id <= 0 {
		return nil, apperror.BadRequestKey("aivideo.id.invalid", nil, "视频任务ID无效")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	task, err := repo.GetTask(ctx, platform, userID, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.task_query_failed", nil, "查询AI视频任务失败", err)
	}
	if task == nil || task.Platform != platform || task.UserID != userID || task.ID != id || task.IsDel != IsDelActive {
		return nil, apperror.NotFoundKey("aivideo.not_found", nil, "AI视频任务不存在")
	}
	return task, nil
}

func (s *Service) engineForTask(ctx context.Context, task *VideoTask) (infraai.VideoEngine, string, *apperror.Error) {
	if task == nil {
		return nil, "", apperror.NotFoundKey("aivideo.not_found", nil, "AI视频任务不存在")
	}
	providerTaskID := strings.TrimSpace(task.ProviderTaskID)
	if providerTaskID == "" {
		return nil, "", apperror.InternalKey("aivideo.provider_task_missing", nil, "AI视频任务尚未绑定Provider任务")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, "", appErr
	}
	agent, appErr := s.validVideoAgent(ctx, repo, task.AgentID)
	if appErr != nil {
		return nil, "", appErr
	}
	engine, appErr := s.engine(ctx, *agent)
	if appErr != nil {
		return nil, "", appErr
	}
	return engine, providerTaskID, nil
}

func (s *Service) validVideoAgent(ctx context.Context, repo Repository, agentID int64) (*AgentRuntime, *apperror.Error) {
	agent, err := repo.AgentForRuntime(ctx, agentID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.agent_query_failed", nil, "查询视频智能体失败", err)
	}
	if agent == nil || agent.AgentID <= 0 {
		return nil, apperror.NotFoundKey("aivideo.agent_not_found", nil, "视频智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !supportsScene(agent.ScenesJSON, capability.SceneVideoGenerate) {
		return nil, apperror.BadRequestKey("aivideo.agent_unavailable", nil, "该智能体不支持视频生成")
	}
	if strings.TrimSpace(agent.EngineAPIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("aivideo.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	return agent, nil
}

func (s *Service) engine(ctx context.Context, agent AgentRuntime) (infraai.VideoEngine, *apperror.Error) {
	if s == nil || s.secretbox == nil {
		return nil, apperror.InternalKey("aivideo.secretbox_missing", nil, "AI密钥服务未配置")
	}
	apiKey, err := s.secretbox.Decrypt(agent.EngineAPIKeyEnc)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.provider_key_decrypt_failed", nil, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequestKey("aivideo.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.InternalKey("aivideo.engine_missing", nil, "AI视频引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewVideoEngine(ctx, EngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.engine_create_failed", nil, "创建AI视频引擎失败", err)
	}
	if engine == nil {
		return nil, apperror.InternalKey("aivideo.engine_missing", nil, "AI视频引擎未配置")
	}
	return engine, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("aivideo.repository_missing", nil, "AI视频生成仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) loadCOSConfig(ctx context.Context, repo Repository) (*cosRuntimeConfig, *apperror.Error) {
	if s == nil || s.secretbox == nil {
		return nil, apperror.InternalKey("aivideo.secretbox_missing", nil, "AI密钥服务未配置")
	}
	cfg, err := repo.LoadUploadConfig(ctx)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.upload_config.read_failed", nil, "读取上传配置失败", err)
	}
	if cfg == nil || strings.TrimSpace(cfg.Driver) != StorageProviderCOS {
		return nil, apperror.InternalKey("aivideo.cos_config.missing", nil, "未配置有效的 COS 上传配置")
	}
	secretID, err := s.secretbox.Decrypt(cfg.SecretIDEnc)
	if err != nil || strings.TrimSpace(secretID) == "" {
		return nil, apperror.InternalKey("aivideo.cos_secret_id.unavailable", nil, "COS SecretID 不可用")
	}
	secretKey, err := s.secretbox.Decrypt(cfg.SecretKeyEnc)
	if err != nil || strings.TrimSpace(secretKey) == "" {
		return nil, apperror.InternalKey("aivideo.cos_secret_key.unavailable", nil, "COS SecretKey 不可用")
	}
	return &cosRuntimeConfig{SecretID: strings.TrimSpace(secretID), SecretKey: strings.TrimSpace(secretKey), Bucket: strings.TrimSpace(cfg.Bucket), Region: strings.TrimSpace(cfg.Region), Endpoint: strings.TrimSpace(cfg.Endpoint), BucketDomain: strings.TrimSpace(cfg.BucketDomain)}, nil
}

type cosRuntimeConfig struct {
	SecretID     string
	SecretKey    string
	Bucket       string
	Region       string
	Endpoint     string
	BucketDomain string
}

func (s *Service) referenceRandomID() (string, error) {
	randomSource := rand.Read
	if s != nil && s.random != nil {
		randomSource = s.random
	}
	buf := make([]byte, 6)
	if _, err := randomSource(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Service) markTaskFailed(platform string, userID int64, id int64, message string) error {
	if s == nil || s.repository == nil || userID <= 0 || id <= 0 {
		return ErrRepositoryNotConfigured
	}
	now := s.now()
	return s.repository.UpdateTask(context.Background(), platform, userID, id, map[string]any{"status": StatusFailed, "error_message": strings.TrimSpace(message), "updated_at": now, "finished_at": now})
}

func (s *Service) failRun(ctx context.Context, runID int64, message string, startedAt time.Time) {
	if s == nil || s.runRecorder == nil || runID <= 0 {
		return
	}
	finishedAt := s.now()
	_ = s.runRecorder.Fail(ctx, airun.FailInput{RunID: runID, Message: strings.TrimSpace(message), FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
}

func (s *Service) finishRunForVideoStatus(ctx context.Context, runID int64, status string, message string, startedAt time.Time) *apperror.Error {
	if s == nil || s.runRecorder == nil {
		return apperror.InternalKey("aivideo.run_recorder_missing", nil, "AI视频运行记录服务未配置")
	}
	if runID <= 0 {
		return apperror.InternalKey("aivideo.run_binding_missing", nil, "AI视频任务缺少运行记录绑定")
	}
	finishedAt := s.now()
	duration := durationMS(startedAt, finishedAt)
	switch status {
	case StatusCompleted:
		if err := s.runRecorder.Complete(ctx, airun.CompleteInput{RunID: runID, FinishedAt: finishedAt, DurationMS: duration}); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.run_complete_failed", nil, "更新AI视频运行记录失败", err)
		}
	case StatusFailed:
		if err := s.runRecorder.Fail(ctx, airun.FailInput{RunID: runID, Message: message, FinishedAt: finishedAt, DurationMS: duration}); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.run_fail_failed", nil, "更新AI视频运行记录失败", err)
		}
	case StatusCancelled:
		if err := s.runRecorder.Cancel(ctx, airun.CancelInput{RunID: runID, Message: message, FinishedAt: finishedAt, DurationMS: duration}); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "aivideo.run_cancel_failed", nil, "更新AI视频运行记录失败", err)
		}
	}
	return nil
}

func videoRunRequestID(taskID int64) string {
	return "ai_video_task_" + strconv.FormatInt(taskID, 10)
}

func durationMS(startedAt time.Time, finishedAt time.Time) uint {
	if startedAt.IsZero() || finishedAt.Before(startedAt) {
		return 0
	}
	return uint(finishedAt.Sub(startedAt).Milliseconds())
}

func normalizeVideoStatus(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "success", "completed", "succeeded":
		return StatusCompleted, true
	case "failed", "failure", "error":
		return StatusFailed, true
	case "cancelled", "canceled":
		return StatusCancelled, true
	case "running", "processing", "in_progress":
		return StatusRunning, true
	case "pending", "queued":
		return StatusPending, true
	default:
		return "", false
	}
}

func isTerminalStatus(status string) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

func isReferenceMediaKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image", "video", "audio":
		return true
	default:
		return false
	}
}

func referenceMimeMatchesKind(kind string, mimeType string) bool {
	prefix := strings.ToLower(strings.TrimSpace(kind)) + "/"
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), prefix)
}

func referenceMimeType(raw string, fileName string, body []byte) string {
	mimeType := strings.TrimSpace(raw)
	if mimeType == "" || strings.EqualFold(mimeType, "application/octet-stream") {
		if ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName))); ext != "" {
			if byExt := mime.TypeByExtension(ext); strings.TrimSpace(byExt) != "" {
				mimeType = strings.TrimSpace(byExt)
			}
		}
	}
	if (mimeType == "" || strings.EqualFold(mimeType, "application/octet-stream")) && len(body) > 0 {
		mimeType = http.DetectContentType(body)
	}
	return strings.TrimSpace(mimeType)
}

func referenceMediaKey(platform string, userID int64, kind string, mimeType string, now time.Time, randomID string) string {
	return fmt.Sprintf("ai-video-references/%s/%s/%04d/%02d/%02d/%d-%s%s", strings.TrimSpace(platform), strings.ToLower(strings.TrimSpace(kind)), now.Year(), int(now.Month()), now.Day(), userID, randomID, extensionForReferenceMime(mimeType))
}

func extensionForReferenceMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	default:
		parts := strings.Split(strings.ToLower(strings.TrimSpace(mimeType)), "/")
		if len(parts) == 2 && parts[1] != "" {
			return "." + strings.ReplaceAll(parts[1], "x-", "")
		}
		return ".bin"
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

func supportsScene(raw string, want string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil || len(scenes) == 0 {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == want {
			return true
		}
	}
	return false
}
