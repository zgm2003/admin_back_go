package aivideo

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type Service struct {
	repository    Repository
	secretbox     Secretbox
	engineFactory EngineFactory
	runRecorder   RunRecorder
	now           func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repository: deps.Repository, secretbox: deps.Secretbox, engineFactory: deps.EngineFactory, runRecorder: deps.RunRecorder, now: now}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.UserID <= 0 || input.AgentID <= 0 || input.Prompt == "" {
		return nil, apperror.BadRequestKey("canvas.ai.video.request.invalid", nil, "视频生成参数错误")
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
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_create_failed", nil, "创建Canvas视频任务失败", err)
	}
	if s.runRecorder == nil {
		if markErr := s.markTaskFailed(input.UserID, taskID, "Canvas视频运行记录服务未配置"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", markErr)
		}
		return nil, apperror.InternalKey("canvas.ai.video.run_recorder_missing", nil, "Canvas视频运行记录服务未配置")
	}
	runStartedAt := now
	runID, err := s.runRecorder.Start(ctx, airun.StartInput{
		Platform:         enum.PlatformCanvas,
		Modality:         enum.AIRunModalityVideo,
		SourceType:       enum.AIRunSourceCanvasVideoTask,
		SourceID:         uint64(taskID),
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
		if markErr := s.markTaskFailed(input.UserID, taskID, "创建Canvas视频运行记录失败"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", markErr)
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.run_start_failed", nil, "创建Canvas视频运行记录失败", err)
	}
	providerTask, err := engine.CreateVideo(ctx, infraai.VideoInput{Model: modelID, Prompt: input.Prompt, DurationSeconds: input.DurationSeconds, Size: input.Size, ResolutionName: input.ResolutionName})
	if err != nil {
		s.failRun(context.Background(), runID, "Canvas视频生成失败", runStartedAt)
		if markErr := s.markTaskFailed(input.UserID, taskID, "Canvas视频生成失败"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", markErr)
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_failed", nil, "Canvas视频生成失败", err)
	}
	providerTaskID := ""
	if providerTask != nil {
		providerTaskID = strings.TrimSpace(providerTask.ID)
	}
	if providerTaskID == "" {
		s.failRun(context.Background(), runID, "Canvas视频任务创建结果无效", runStartedAt)
		if markErr := s.markTaskFailed(input.UserID, taskID, "Canvas视频任务创建结果无效"); markErr != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", markErr)
		}
		return nil, apperror.InternalKey("canvas.ai.video.provider_task_invalid", nil, "Canvas视频任务创建结果无效")
	}
	status, ok := normalizeVideoStatus(providerTask.Status)
	if !ok {
		s.failRun(context.Background(), runID, "Canvas视频任务状态无效", runStartedAt)
		return nil, apperror.InternalKey("canvas.ai.video.provider_status_invalid", nil, "Canvas视频任务状态无效")
	}
	fields := map[string]any{"provider_task_id": providerTaskID, "provider_id": agent.ProviderID, "model_id": modelID, "status": status, "updated_at": s.now()}
	if isTerminalStatus(status) {
		fields["finished_at"] = s.now()
	}
	if err := repo.UpdateTask(ctx, input.UserID, taskID, fields); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", err)
	}
	if isTerminalStatus(status) {
		if appErr := s.finishRunForVideoStatus(context.Background(), taskID, status, "", runStartedAt); appErr != nil {
			return nil, appErr
		}
	}
	return &CreateResponse{ID: taskID, Status: status}, nil
}

func (s *Service) Status(ctx context.Context, userID int64, id int64) (*StatusResponse, *apperror.Error) {
	task, appErr := s.ownedTask(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	engine, providerTaskID, appErr := s.engineForTask(ctx, task)
	if appErr != nil {
		return nil, appErr
	}
	providerTask, err := engine.GetVideo(ctx, providerTaskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_status_failed", nil, "查询Canvas视频任务失败", err)
	}
	if providerTask == nil {
		return nil, apperror.InternalKey("canvas.ai.video.provider_status_invalid", nil, "Canvas视频任务状态无效")
	}
	status, ok := normalizeVideoStatus(providerTask.Status)
	if !ok {
		return nil, apperror.InternalKey("canvas.ai.video.provider_status_invalid", nil, "Canvas视频任务状态无效")
	}
	fields := map[string]any{"status": status, "error_message": strings.TrimSpace(providerTask.ErrorMessage), "updated_at": s.now()}
	if isTerminalStatus(status) {
		fields["finished_at"] = s.now()
	}
	if err := s.repository.UpdateTask(ctx, userID, id, fields); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", err)
	}
	if isTerminalStatus(status) {
		if appErr := s.finishRunForVideoStatus(context.Background(), id, status, providerTask.ErrorMessage, task.CreatedAt); appErr != nil {
			return nil, appErr
		}
	}
	return &StatusResponse{ID: task.ID, Status: status}, nil
}

func (s *Service) Content(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error) {
	task, appErr := s.ownedTask(ctx, userID, id)
	if appErr != nil {
		return nil, "", appErr
	}
	engine, providerTaskID, appErr := s.engineForTask(ctx, task)
	if appErr != nil {
		return nil, "", appErr
	}
	body, contentType, err := engine.DownloadVideo(ctx, providerTaskID)
	if err != nil {
		return nil, "", apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_content_failed", nil, "下载Canvas视频内容失败", err)
	}
	if len(body) == 0 {
		return nil, "", apperror.BadRequestKey("canvas.ai.video.content_empty", nil, "Canvas视频内容为空")
	}
	return body, contentType, nil
}

func (s *Service) ownedTask(ctx context.Context, userID int64, id int64) (*VideoTask, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if id <= 0 {
		return nil, apperror.BadRequestKey("canvas.ai.video.id.invalid", nil, "视频任务ID无效")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	task, err := repo.GetTask(ctx, userID, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_query_failed", nil, "查询Canvas视频任务失败", err)
	}
	if task == nil || task.UserID != userID || task.ID != id || task.IsDel != IsDelActive {
		return nil, apperror.NotFoundKey("canvas.ai.video.not_found", nil, "Canvas视频任务不存在")
	}
	return task, nil
}

func (s *Service) engineForTask(ctx context.Context, task *VideoTask) (infraai.VideoEngine, string, *apperror.Error) {
	if task == nil {
		return nil, "", apperror.NotFoundKey("canvas.ai.video.not_found", nil, "Canvas视频任务不存在")
	}
	providerTaskID := strings.TrimSpace(task.ProviderTaskID)
	if providerTaskID == "" {
		return nil, "", apperror.InternalKey("canvas.ai.video.provider_task_missing", nil, "Canvas视频任务尚未绑定Provider任务")
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
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.agent_query_failed", nil, "查询视频智能体失败", err)
	}
	if agent == nil || agent.AgentID <= 0 {
		return nil, apperror.NotFoundKey("canvas.ai.video.agent_not_found", nil, "视频智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !supportsScene(agent.ScenesJSON, SceneCanvasVideoGenerate) {
		return nil, apperror.BadRequestKey("canvas.ai.video.agent_unavailable", nil, "该智能体不支持视频生成")
	}
	if strings.TrimSpace(agent.EngineAPIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("canvas.ai.video.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	return agent, nil
}

func (s *Service) engine(ctx context.Context, agent AgentRuntime) (infraai.VideoEngine, *apperror.Error) {
	if s == nil || s.secretbox == nil {
		return nil, apperror.InternalKey("canvas.ai.video.secretbox_missing", nil, "AI密钥服务未配置")
	}
	apiKey, err := s.secretbox.Decrypt(agent.EngineAPIKeyEnc)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_key_decrypt_failed", nil, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequestKey("canvas.ai.video.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.InternalKey("canvas.ai.video.engine_missing", nil, "AI视频引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewVideoEngine(ctx, EngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.engine_create_failed", nil, "创建AI视频引擎失败", err)
	}
	if engine == nil {
		return nil, apperror.InternalKey("canvas.ai.video.engine_missing", nil, "AI视频引擎未配置")
	}
	return engine, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("canvas.ai.video.repository_missing", nil, "Canvas视频生成仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) markTaskFailed(userID int64, id int64, message string) error {
	if s == nil || s.repository == nil || userID <= 0 || id <= 0 {
		return ErrRepositoryNotConfigured
	}
	now := s.now()
	return s.repository.UpdateTask(context.Background(), userID, id, map[string]any{"status": StatusFailed, "error_message": strings.TrimSpace(message), "updated_at": now, "finished_at": now})
}

func (s *Service) failRun(ctx context.Context, runID int64, message string, startedAt time.Time) {
	if s == nil || s.runRecorder == nil || runID <= 0 {
		return
	}
	finishedAt := s.now()
	_ = s.runRecorder.Fail(ctx, airun.FailInput{RunID: runID, Message: strings.TrimSpace(message), FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
}

func (s *Service) finishRunForVideoStatus(ctx context.Context, taskID int64, status string, message string, startedAt time.Time) *apperror.Error {
	if s == nil || s.runRecorder == nil {
		return apperror.InternalKey("canvas.ai.video.run_recorder_missing", nil, "Canvas视频运行记录服务未配置")
	}
	finishedAt := s.now()
	duration := durationMS(startedAt, finishedAt)
	sourceID := uint64(taskID)
	switch status {
	case StatusCompleted:
		if err := s.runRecorder.CompleteSource(ctx, airun.CompleteSourceInput{SourceType: enum.AIRunSourceCanvasVideoTask, SourceID: sourceID, UsageStatus: enum.AIRunUsageUnavailable, FinishedAt: finishedAt, DurationMS: duration}); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.run_complete_failed", nil, "更新Canvas视频运行记录失败", err)
		}
	case StatusFailed:
		if err := s.runRecorder.FailSource(ctx, airun.FailSourceInput{SourceType: enum.AIRunSourceCanvasVideoTask, SourceID: sourceID, Message: message, FinishedAt: finishedAt, DurationMS: duration}); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.run_fail_failed", nil, "更新Canvas视频运行记录失败", err)
		}
	case StatusCancelled:
		if err := s.runRecorder.CancelSource(ctx, airun.CancelSourceInput{SourceType: enum.AIRunSourceCanvasVideoTask, SourceID: sourceID, Message: message, FinishedAt: finishedAt, DurationMS: duration}); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.run_cancel_failed", nil, "更新Canvas视频运行记录失败", err)
		}
	}
	return nil
}

func videoRunRequestID(taskID int64) string {
	return "canvas_video_task_" + strconv.FormatInt(taskID, 10)
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
