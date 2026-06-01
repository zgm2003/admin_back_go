package canvas

import (
	"context"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type VideoRuntimeService struct {
	repository    VideoRepository
	secretbox     Secretbox
	engineFactory VideoEngineFactory
}

type VideoRuntimeDependencies struct {
	Repository    VideoRepository
	Secretbox     Secretbox
	EngineFactory VideoEngineFactory
}

func NewVideoRuntimeService(deps VideoRuntimeDependencies) *VideoRuntimeService {
	return &VideoRuntimeService{repository: deps.Repository, secretbox: deps.Secretbox, engineFactory: deps.EngineFactory}
}

func (s *VideoRuntimeService) Create(ctx context.Context, input VideoCreateInput) (*VideoCreateResult, *apperror.Error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.UserID <= 0 || input.AgentID <= 0 || input.Prompt == "" {
		return nil, apperror.BadRequestKey("canvas.ai.video.request.invalid", nil, "视频生成参数错误")
	}
	agent, appErr := s.validVideoAgent(ctx, input.AgentID)
	if appErr != nil {
		return nil, appErr
	}
	engine, appErr := s.engine(ctx, agent)
	if appErr != nil {
		return nil, appErr
	}
	modelID := strings.TrimSpace(agent.ModelID)
	now := time.Now()
	videoTask := VideoTask{UserID: input.UserID, AgentID: input.AgentID, ProviderID: agent.ProviderID, ModelID: modelID, Prompt: input.Prompt, DurationSeconds: input.DurationSeconds, Size: input.Size, ResolutionName: input.ResolutionName, Status: "pending", IsDel: IsDelActive, CreatedAt: now, UpdatedAt: now}
	taskID, err := s.repository.CreateVideoTask(ctx, videoTask)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_create_failed", nil, "创建Canvas视频任务失败", err)
	}
	videoTask.ID = taskID
	task, err := engine.CreateVideo(ctx, infraai.VideoInput{
		Model: modelID, Prompt: input.Prompt, DurationSeconds: input.DurationSeconds,
		Size: input.Size, ResolutionName: input.ResolutionName,
	})
	if err != nil {
		_ = s.repository.UpdateVideoTask(context.Background(), input.UserID, taskID, map[string]any{"status": "failed", "error_message": "Canvas视频生成失败", "updated_at": time.Now(), "finished_at": time.Now()})
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_failed", nil, "Canvas视频生成失败", err)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		_ = s.repository.UpdateVideoTask(context.Background(), input.UserID, taskID, map[string]any{"status": "failed", "error_message": "Canvas视频任务创建结果无效", "updated_at": time.Now(), "finished_at": time.Now()})
		return nil, apperror.InternalKey("canvas.ai.video.provider_task_invalid", nil, "Canvas视频任务创建结果无效")
	}
	providerTaskID := strings.TrimSpace(task.ID)
	status := normalizeVideoStatus(task.Status)
	fields := map[string]any{"provider_task_id": providerTaskID, "provider_id": agent.ProviderID, "model_id": modelID, "status": status, "updated_at": time.Now()}
	if status == "completed" || status == "failed" || status == "cancelled" {
		fields["finished_at"] = time.Now()
	}
	if err := s.repository.UpdateVideoTask(ctx, input.UserID, taskID, fields); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_update_failed", nil, "更新Canvas视频任务失败", err)
	}
	return &VideoCreateResult{ID: taskID, ProviderID: agent.ProviderID, ModelID: modelID, ProviderTaskID: providerTaskID, Status: status}, nil
}

func (s *VideoRuntimeService) Task(ctx context.Context, userID int64, id int64) (*VideoTask, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("canvas.ai.video.repository_missing", nil, "Canvas视频生成仓储未配置")
	}
	task, err := s.repository.GetVideoTask(ctx, userID, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.task_query_failed", nil, "查询Canvas视频任务失败", err)
	}
	return task, nil
}

func (s *VideoRuntimeService) Status(ctx context.Context, input VideoStatusInput) (*VideoProviderStatus, *apperror.Error) {
	engine, taskID, appErr := s.engineForTask(ctx, input.Task)
	if appErr != nil {
		return nil, appErr
	}
	task, err := engine.GetVideo(ctx, taskID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_status_failed", nil, "查询Canvas视频任务失败", err)
	}
	if task == nil {
		return nil, apperror.InternalKey("canvas.ai.video.provider_status_invalid", nil, "Canvas视频任务状态无效")
	}
	status := normalizeVideoStatus(task.Status)
	fields := map[string]any{"status": status, "error_message": strings.TrimSpace(task.ErrorMessage), "updated_at": time.Now()}
	if status == "completed" || status == "failed" || status == "cancelled" {
		fields["finished_at"] = time.Now()
	}
	_ = s.repository.UpdateVideoTask(ctx, input.UserID, input.Task.ID, fields)
	return &VideoProviderStatus{Status: status, ErrorMessage: task.ErrorMessage}, nil
}

func (s *VideoRuntimeService) Content(ctx context.Context, input VideoContentInput) ([]byte, string, *apperror.Error) {
	engine, taskID, appErr := s.engineForTask(ctx, input.Task)
	if appErr != nil {
		return nil, "", appErr
	}
	body, contentType, err := engine.DownloadVideo(ctx, taskID)
	if err != nil {
		return nil, "", apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_content_failed", nil, "下载Canvas视频内容失败", err)
	}
	return body, contentType, nil
}

func (s *VideoRuntimeService) engineForTask(ctx context.Context, task *VideoTask) (infraai.VideoEngine, string, *apperror.Error) {
	if task == nil {
		return nil, "", apperror.NotFoundKey("canvas.ai.video.not_found", nil, "Canvas视频任务不存在")
	}
	taskID := strings.TrimSpace(task.ProviderTaskID)
	if taskID == "" {
		return nil, "", missingVideoTaskID()
	}
	agent, appErr := s.validVideoAgent(ctx, task.AgentID)
	if appErr != nil {
		return nil, "", appErr
	}
	engine, appErr := s.engine(ctx, agent)
	if appErr != nil {
		return nil, "", appErr
	}
	return engine, taskID, nil
}

func (s *VideoRuntimeService) validVideoAgent(ctx context.Context, agentID int64) (*VideoAgentRuntime, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("canvas.ai.video.repository_missing", nil, "Canvas视频生成仓储未配置")
	}
	agent, err := s.repository.AgentForVideoRuntime(ctx, agentID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.agent_query_failed", nil, "查询视频智能体失败", err)
	}
	if agent == nil || agent.AgentID <= 0 {
		return nil, apperror.NotFoundKey("canvas.ai.video.agent_not_found", nil, "视频智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !supportsScene(agent.ScenesJSON, canvasVideoAgentScene) {
		return nil, apperror.BadRequestKey("canvas.ai.video.agent_unavailable", nil, "该智能体不支持视频生成")
	}
	if strings.TrimSpace(agent.EngineAPIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("canvas.ai.video.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	return agent, nil
}

func (s *VideoRuntimeService) engine(ctx context.Context, agent *VideoAgentRuntime) (infraai.VideoEngine, *apperror.Error) {
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
	engine, err := s.engineFactory.NewVideoEngine(ctx, VideoEngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.engine_create_failed", nil, "创建AI视频引擎失败", err)
	}
	if engine == nil {
		return nil, apperror.InternalKey("canvas.ai.video.engine_missing", nil, "AI视频引擎未配置")
	}
	return engine, nil
}

func missingVideoTaskID() *apperror.Error {
	return apperror.BadRequestKey("canvas.ai.video.provider_task_missing", nil, "Canvas视频任务尚未绑定Provider任务")
}
