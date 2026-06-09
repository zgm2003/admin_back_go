package canvas

import (
	"context"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

var publicCanvasScenes = []string{
	canvasTextAgentScene,
	canvasImageAgentScene,
	canvasVideoAgentScene,
	canvasAudioAgentScene,
}

const (
	canvasTextAgentScene  = "canvas_text_generate"
	canvasImageAgentScene = "canvas_image_generate"
	canvasVideoAgentScene = "canvas_video_generate"
	canvasAudioAgentScene = "canvas_audio_generate"
)

type AuthPolicyService interface {
	AllowRegister(ctx context.Context, platform string) (bool, error)
}

type SettingsDependencies struct {
	AuthPolicy AuthPolicyService
}

type Service struct {
	repository Repository
	settings   SettingsDependencies
}

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func NewServiceWithSettings(repository Repository, deps SettingsDependencies) *Service {
	return &Service{repository: repository, settings: deps}
}

func (s *Service) PublicSettings(ctx context.Context, input SettingsInput) (*SettingsResponse, *apperror.Error) {
	allowRegister, appErr := s.canvasAllowRegister(ctx)
	if appErr != nil {
		return nil, appErr
	}
	result := &SettingsResponse{
		AllowRegister: allowRegister,
		Scenes:        append([]string(nil), publicCanvasScenes...),
	}
	agents, appErr := s.canvasAgentGroups(ctx)
	if appErr != nil {
		return nil, appErr
	}
	result.Agents = agents
	return result, nil
}

func (s *Service) canvasAllowRegister(ctx context.Context) (bool, *apperror.Error) {
	if s == nil || s.settings.AuthPolicy == nil {
		return false, nil
	}
	allowed, err := s.settings.AuthPolicy.AllowRegister(ctx, enum.PlatformCanvas)
	if err != nil {
		return false, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.settings.auth_policy_failed", nil, "读取Canvas注册策略失败", err)
	}
	return allowed, nil
}

func (s *Service) canvasAgentGroups(ctx context.Context) (CanvasAgentGroups, *apperror.Error) {
	text, appErr := s.canvasAgentsByScene(ctx, canvasTextAgentScene)
	if appErr != nil {
		return CanvasAgentGroups{}, appErr
	}
	image, appErr := s.canvasAgentsByScene(ctx, canvasImageAgentScene)
	if appErr != nil {
		return CanvasAgentGroups{}, appErr
	}
	video, appErr := s.canvasAgentsByScene(ctx, canvasVideoAgentScene)
	if appErr != nil {
		return CanvasAgentGroups{}, appErr
	}
	audio, appErr := s.canvasAgentsByScene(ctx, canvasAudioAgentScene)
	if appErr != nil {
		return CanvasAgentGroups{}, appErr
	}
	return CanvasAgentGroups{Text: text, Image: image, Video: video, Audio: audio}, nil
}

func (s *Service) canvasAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, *apperror.Error) {
	agents, err := s.repo().ListAgentsByScene(ctx, scene)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.settings.agents_query_failed", nil, "查询Canvas智能体配置失败", err)
	}
	if agents == nil {
		return []CanvasAgentOption{}, nil
	}
	return agents, nil
}

func (s *Service) repo() Repository {
	if s == nil || s.repository == nil {
		return failingRepository{}
	}
	return s.repository
}

type failingRepository struct{}

func (failingRepository) ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error) {
	return nil, ErrRepositoryNotConfigured
}
