package aiaudio

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/capability"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const (
	defaultVoice          = "alloy"
	defaultResponseFormat = "mp3"
)

var allowedVoices = map[string]struct{}{
	"alloy": {}, "ash": {}, "ballad": {}, "coral": {}, "echo": {}, "fable": {}, "nova": {}, "onyx": {}, "sage": {}, "shimmer": {}, "verse": {}, "marin": {}, "cedar": {},
}

var allowedResponseFormats = map[string]struct{}{
	"mp3": {}, "wav": {}, "opus": {}, "aac": {}, "flac": {}, "pcm": {},
}

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

func (s *Service) Generate(ctx context.Context, input GenerateInput) (*GenerateResponse, *apperror.Error) {
	normalized, appErr := normalizeInput(input)
	if appErr != nil {
		return nil, appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agent, appErr := s.validAudioAgent(ctx, repo, normalized.AgentID)
	if appErr != nil {
		return nil, appErr
	}
	engine, appErr := s.engine(ctx, *agent)
	if appErr != nil {
		return nil, appErr
	}
	if s.runRecorder == nil {
		return nil, apperror.InternalKey("aiaudio.run_recorder_missing", nil, "AI音频运行记录服务未配置")
	}
	startedAt := s.now()
	runID, err := s.runRecorder.Start(ctx, airun.StartInput{
		Platform:         normalized.Platform,
		RequestID:        audioRunRequestID(startedAt),
		UserID:           normalized.UserID,
		AgentID:          normalized.AgentID,
		ProviderID:       agent.ProviderID,
		ModelID:          strings.TrimSpace(agent.ModelID),
		ModelDisplayName: agent.ModelDisplayName,
		InputSnapshot:    normalized.Prompt,
		StartedAt:        startedAt,
	})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiaudio.run_start_failed", nil, "创建AI音频运行记录失败", err)
	}
	result, err := engine.GenerateAudio(ctx, infraai.AudioInput{
		Model:          strings.TrimSpace(agent.ModelID),
		Prompt:         normalized.Prompt,
		Voice:          normalized.Voice,
		ResponseFormat: normalized.ResponseFormat,
		Speed:          normalized.Speed,
		Instructions:   normalized.Instructions,
	})
	if err != nil {
		finishedAt := s.now()
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: "AI音频生成失败", FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiaudio.provider_failed", nil, "AI音频生成失败", err)
	}
	if result == nil || len(result.Body) == 0 {
		finishedAt := s.now()
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: "AI音频内容为空", FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, apperror.BadRequestKey("aiaudio.content_empty", nil, "AI音频内容为空")
	}
	contentType, appErr := normalizeContentType(result.ContentType, normalized.ResponseFormat)
	if appErr != nil {
		finishedAt := s.now()
		_ = s.runRecorder.Fail(context.Background(), airun.FailInput{RunID: runID, Message: appErr.Message, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)})
		return nil, appErr
	}
	finishedAt := s.now()
	if err := s.runRecorder.Complete(context.Background(), airun.CompleteInput{RunID: runID, FinishedAt: finishedAt, DurationMS: durationMS(startedAt, finishedAt)}); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiaudio.run_complete_failed", nil, "更新AI音频运行记录失败", err)
	}
	return &GenerateResponse{Body: result.Body, ContentType: contentType}, nil
}

func normalizeInput(input GenerateInput) (GenerateInput, *apperror.Error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.Voice = strings.ToLower(strings.TrimSpace(input.Voice))
	input.ResponseFormat = strings.ToLower(strings.TrimSpace(input.ResponseFormat))
	input.Instructions = strings.TrimSpace(input.Instructions)
	if !enum.IsRegisteredPlatform(input.Platform) {
		return input, apperror.BadRequestKey("aiaudio.platform.invalid", nil, "无效的音频生成平台")
	}
	if input.UserID <= 0 {
		return input, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.AgentID <= 0 || input.Prompt == "" {
		return input, apperror.BadRequestKey("aiaudio.request.invalid", nil, "音频生成参数错误")
	}
	if input.Voice == "" {
		input.Voice = defaultVoice
	}
	if _, ok := allowedVoices[input.Voice]; !ok {
		return input, apperror.BadRequestKey("aiaudio.request.invalid", nil, "音频生成参数错误")
	}
	if input.ResponseFormat == "" {
		input.ResponseFormat = defaultResponseFormat
	}
	if _, ok := allowedResponseFormats[input.ResponseFormat]; !ok {
		return input, apperror.BadRequestKey("aiaudio.request.invalid", nil, "音频生成参数错误")
	}
	if input.Speed != nil && (*input.Speed < 0.25 || *input.Speed > 4) {
		return input, apperror.BadRequestKey("aiaudio.request.invalid", nil, "音频生成参数错误")
	}
	if len(input.Instructions) > 4000 {
		return input, apperror.BadRequestKey("aiaudio.request.invalid", nil, "音频生成参数错误")
	}
	return input, nil
}

func (s *Service) validAudioAgent(ctx context.Context, repo Repository, agentID int64) (*AgentRuntime, *apperror.Error) {
	agent, err := repo.AgentForRuntime(ctx, agentID)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiaudio.agent_query_failed", nil, "查询音频智能体失败", err)
	}
	if agent == nil || agent.AgentID <= 0 {
		return nil, apperror.NotFoundKey("aiaudio.agent_not_found", nil, "音频智能体不存在")
	}
	if agent.AgentStatus != enum.CommonYes || agent.EngineStatus != enum.CommonYes || !supportsScene(agent.ScenesJSON, capability.SceneAudioGenerate) {
		return nil, apperror.BadRequestKey("aiaudio.agent_unavailable", nil, "该智能体不支持音频生成")
	}
	if strings.TrimSpace(agent.EngineAPIKeyEnc) == "" {
		return nil, apperror.BadRequestKey("aiaudio.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	if strings.TrimSpace(agent.ModelID) == "" {
		return nil, apperror.BadRequestKey("aiaudio.model_missing", nil, "音频智能体模型未配置")
	}
	return agent, nil
}

func (s *Service) engine(ctx context.Context, agent AgentRuntime) (infraai.AudioEngine, *apperror.Error) {
	if s == nil || s.secretbox == nil {
		return nil, apperror.InternalKey("aiaudio.secretbox_missing", nil, "AI密钥服务未配置")
	}
	apiKey, err := s.secretbox.Decrypt(agent.EngineAPIKeyEnc)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiaudio.provider_key_decrypt_failed", nil, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequestKey("aiaudio.provider_key_missing", nil, "AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.InternalKey("aiaudio.engine_missing", nil, "AI音频引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewAudioEngine(ctx, EngineConfig{EngineType: infraai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiaudio.engine_create_failed", nil, "创建AI音频引擎失败", err)
	}
	if engine == nil {
		return nil, apperror.InternalKey("aiaudio.engine_missing", nil, "AI音频引擎未配置")
	}
	return engine, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("aiaudio.repository_missing", nil, "AI音频生成仓储未配置")
	}
	return s.repository, nil
}

func normalizeContentType(contentType string, format string) (string, *apperror.Error) {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" || strings.Contains(strings.ToLower(contentType), "octet-stream") {
		return audioContentType(format), nil
	}
	if strings.HasPrefix(strings.ToLower(contentType), "audio/") {
		return contentType, nil
	}
	return "", apperror.BadRequestKey("aiaudio.content_type_invalid", nil, "AI音频内容类型无效")
}

func audioContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/opus"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/pcm"
	default:
		return "audio/mpeg"
	}
}

func audioRunRequestID(now time.Time) string {
	return "ai_audio_" + strconv.FormatInt(now.UnixNano(), 10)
}

func durationMS(startedAt time.Time, finishedAt time.Time) uint {
	if startedAt.IsZero() || finishedAt.Before(startedAt) {
		return 0
	}
	return uint(finishedAt.Sub(startedAt).Milliseconds())
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
