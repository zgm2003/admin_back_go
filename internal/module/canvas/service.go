package canvas

import (
	"context"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

var publicCanvasScenes = []string{
	canvasTextAgentScene,
	canvasImageAgentScene,
	canvasVideoAgentScene,
}

const (
	canvasTextAgentScene  = "canvas_text_generate"
	canvasImageAgentScene = "canvas_image_generate"
	canvasVideoAgentScene = "canvas_video_generate"
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

func (s *Service) ListPrompts(ctx context.Context, query PromptListQuery) (*PromptListResponse, *apperror.Error) {
	rows, total, err := s.repo().ListPrompts(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.prompt.query_failed", nil, "查询Canvas提示词失败", err)
	}
	query = normalizePromptListQuery(query)
	return &PromptListResponse{List: promptItems(rows), Page: page(query.CurrentPage, query.PageSize, total)}, nil
}

func (s *Service) PublicPrompts(ctx context.Context, query PromptListQuery) (*PromptListResponse, *apperror.Error) {
	query.Status = StatusEnabled
	query.IsDel = IsDelActive
	return s.ListPrompts(ctx, query)
}

func (s *Service) CreatePrompt(ctx context.Context, input PromptInput) (int64, *apperror.Error) {
	input = normalizePromptInput(input)
	if input.Slug == "" || input.Title == "" || input.Prompt == "" {
		return 0, apperror.BadRequestKey("canvas.prompt.request.invalid", nil, "提示词参数错误")
	}
	row := Prompt{Slug: input.Slug, Category: input.Category, Title: input.Title, CoverURL: input.CoverURL, Prompt: input.Prompt, Preview: input.Preview, TagsJSON: input.TagsJSON, SourceURL: input.SourceURL, Status: normalizeStatus(input.Status), IsDel: IsDelActive}
	id, err := s.repo().CreatePrompt(ctx, row)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.prompt.create_failed", nil, "创建Canvas提示词失败", err)
	}
	return id, nil
}

func (s *Service) ListAssets(ctx context.Context, query AssetListQuery) (*AssetListResponse, *apperror.Error) {
	rows, total, err := s.repo().ListAssets(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.asset.query_failed", nil, "查询Canvas素材失败", err)
	}
	query = normalizeAssetListQuery(query)
	return &AssetListResponse{List: assetItems(rows), Page: page(query.CurrentPage, query.PageSize, total)}, nil
}

func (s *Service) PublicAssets(ctx context.Context, query AssetListQuery) (*AssetListResponse, *apperror.Error) {
	query.Status = StatusEnabled
	query.IsDel = IsDelActive
	return s.ListAssets(ctx, query)
}

func (s *Service) CreateAsset(ctx context.Context, input AssetInput) (int64, *apperror.Error) {
	input = normalizeAssetInput(input)
	if input.Slug == "" || input.Title == "" || !isAssetType(input.Type) {
		return 0, apperror.BadRequestKey("canvas.asset.request.invalid", nil, "素材参数错误")
	}
	row := Asset{Slug: input.Slug, Type: input.Type, Category: input.Category, Title: input.Title, CoverURL: input.CoverURL, Description: input.Description, Content: input.Content, URL: input.URL, TagsJSON: input.TagsJSON, Status: normalizeStatus(input.Status), IsDel: IsDelActive}
	id, err := s.repo().CreateAsset(ctx, row)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "canvas.asset.create_failed", nil, "创建Canvas素材失败", err)
	}
	return id, nil
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
	return CanvasAgentGroups{Text: text, Image: image, Video: video}, nil
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

func (failingRepository) ListPrompts(ctx context.Context, query PromptListQuery) ([]Prompt, int64, error) {
	return nil, 0, ErrRepositoryNotConfigured
}
func (failingRepository) CreatePrompt(ctx context.Context, row Prompt) (int64, error) {
	return 0, ErrRepositoryNotConfigured
}
func (failingRepository) SoftDeletePrompt(ctx context.Context, id int64) error {
	return ErrRepositoryNotConfigured
}
func (failingRepository) ListAssets(ctx context.Context, query AssetListQuery) ([]Asset, int64, error) {
	return nil, 0, ErrRepositoryNotConfigured
}
func (failingRepository) CreateAsset(ctx context.Context, row Asset) (int64, error) {
	return 0, ErrRepositoryNotConfigured
}
func (failingRepository) SoftDeleteAsset(ctx context.Context, id int64) error {
	return ErrRepositoryNotConfigured
}
func (failingRepository) ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error) {
	return nil, ErrRepositoryNotConfigured
}

func normalizePromptInput(input PromptInput) PromptInput {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	input.Prompt = strings.TrimSpace(input.Prompt)
	return input
}
func normalizeAssetInput(input AssetInput) AssetInput {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Type = strings.TrimSpace(input.Type)
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	return input
}
func normalizeStatus(status int) int {
	if status == StatusDisabled {
		return StatusDisabled
	}
	return StatusEnabled
}
func isAssetType(value string) bool { return value == AssetTypeText || value == AssetTypeImage }

func promptItems(rows []Prompt) []PromptItem {
	items := make([]PromptItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, PromptItem{ID: r.ID, Slug: r.Slug, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Prompt: r.Prompt, Preview: r.Preview, TagsJSON: r.TagsJSON, SourceURL: r.SourceURL, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)})
	}
	return items
}
func assetItems(rows []Asset) []AssetItem {
	items := make([]AssetItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, AssetItem{ID: r.ID, Slug: r.Slug, Type: r.Type, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Description: r.Description, Content: r.Content, URL: r.URL, TagsJSON: r.TagsJSON, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)})
	}
	return items
}
func page(current, size int, total int64) Page {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	totalPage := int64(0)
	if size > 0 {
		totalPage = (total + int64(size) - 1) / int64(size)
	}
	return Page{CurrentPage: current, PageSize: size, Total: total, TotalPage: int(totalPage)}
}
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
