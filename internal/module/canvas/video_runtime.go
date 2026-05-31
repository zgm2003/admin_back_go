package canvas

import (
	"context"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	aibilling "admin_back_go/internal/module/ai/billing"
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
	modelID := strings.TrimSpace(input.ModelID)
	if modelID == "" {
		modelID = agent.ModelID
	}
	task, err := engine.CreateVideo(ctx, infraai.VideoInput{
		Model: modelID, Prompt: input.Prompt, DurationSeconds: input.DurationSeconds,
		Size: input.Size, ResolutionName: input.ResolutionName,
	})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_failed", nil, "Canvas视频生成失败", err)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return nil, apperror.InternalKey("canvas.ai.video.provider_task_invalid", nil, "Canvas视频任务创建结果无效")
	}
	task.ID = strings.TrimSpace(task.ID)
	return &VideoCreateResult{ProviderID: agent.ProviderID, ModelID: modelID, ProviderTaskID: task.ID, Status: task.Status}, nil
}

func (s *VideoRuntimeService) Status(ctx context.Context, input VideoStatusInput) (*VideoProviderStatus, *apperror.Error) {
	engine, taskID, appErr := s.engineForRecord(ctx, input.BillingRecord)
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
	return &VideoProviderStatus{Status: task.Status, ErrorMessage: task.ErrorMessage}, nil
}

func (s *VideoRuntimeService) Content(ctx context.Context, input VideoContentInput) ([]byte, string, *apperror.Error) {
	engine, taskID, appErr := s.engineForRecord(ctx, input.BillingRecord)
	if appErr != nil {
		return nil, "", appErr
	}
	body, contentType, err := engine.DownloadVideo(ctx, taskID)
	if err != nil {
		return nil, "", apperror.WrapKey(apperror.CodeInternal, 500, "canvas.ai.video.provider_content_failed", nil, "下载Canvas视频内容失败", err)
	}
	return body, contentType, nil
}

func (s *VideoRuntimeService) engineForRecord(ctx context.Context, record *aibilling.BillingRecord) (infraai.VideoEngine, string, *apperror.Error) {
	if record == nil {
		return nil, "", apperror.NotFoundKey("canvas.ai.video.not_found", nil, "Canvas视频任务不存在")
	}
	taskID := strings.TrimSpace(record.ProviderTaskID)
	if taskID == "" {
		return nil, "", missingVideoTaskID()
	}
	agent, appErr := s.agentForRecord(ctx, record.AgentID)
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
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !supportsScene(agent.ScenesJSON, "image_generate") {
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

func (s *VideoRuntimeService) agentForRecord(ctx context.Context, agentID int64) (*VideoAgentRuntime, *apperror.Error) {
	if agentID <= 0 {
		return nil, apperror.BadRequestKey("canvas.ai.video.agent_not_found", nil, "视频智能体不存在")
	}
	return s.validVideoAgent(ctx, agentID)
}

func missingVideoTaskID() *apperror.Error {
	return apperror.BadRequestKey("canvas.ai.video.provider_task_missing", nil, "Canvas视频任务尚未绑定Provider任务")
}
