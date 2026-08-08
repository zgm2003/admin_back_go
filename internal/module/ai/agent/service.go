package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	sharedmoney "admin_back_go/internal/shared/money"
	"admin_back_go/internal/shared/uploadpolicy"
)

const (
	timeLayout            = "2006-01-02 15:04:05"
	sceneChat             = "chat"
	sceneAgentGenerate    = "agent_generate"
	supportedImageModelID = "gpt-image-2"
)

var sceneLabels = map[string]string{
	sceneChat:                     "对话",
	sceneAgentGenerate:            "工具生成",
	capability.SceneTextGenerate:  "文本生成",
	capability.SceneImageGenerate: "图片生成",
}

type Service struct {
	repository      Repository
	secretbox       secretbox.Box
	tester          ConnectionTester
	pricingResolver officialmodel.Resolver
	capabilities    infraai.TransportCapabilityResolver
	uploadRules     uploadpolicy.Resolver
	contextProfiles ContextProfileResolver
}

type ContextProfileResolver interface {
	RequireAssignable(context.Context, uint64) error
	RequireAgentProfileChangeAllowed(context.Context, uint64, *uint64) error
}

type contextProfileAssignmentCommitter interface {
	ContextProfileAssignmentCommitted(context.Context, uint64, uint64) error
}

type Option func(*Service)

func WithPricingResolver(resolver officialmodel.Resolver) Option {
	return func(service *Service) { service.pricingResolver = resolver }
}

func WithTransportCapabilityResolver(resolver infraai.TransportCapabilityResolver) Option {
	return func(service *Service) { service.capabilities = resolver }
}

func WithUploadRuleResolver(resolver uploadpolicy.Resolver) Option {
	return func(service *Service) { service.uploadRules = resolver }
}

func WithContextProfileResolver(resolver ContextProfileResolver) Option {
	return func(service *Service) { service.contextProfiles = resolver }
}

func NewService(repository Repository, box secretbox.Box, tester ConnectionTester, options ...Option) *Service {
	service := &Service{
		repository: repository, secretbox: box, tester: tester,
		capabilities: infraai.TransportCapabilityResolverFunc(infraai.DefaultTransportCapabilities),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) PageInit(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	attachmentPolicy := s.resolveRequestAttachmentPolicy(ctx)
	connections, err := repo.ListActiveProviders(ctx)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商选项失败", err)
	}
	options := make([]EngineOption, 0, len(connections))
	modelOptions := []ModelOption{}
	for _, row := range connections {
		options = append(options, EngineOption{Label: row.Name, Value: row.ID, EngineType: row.EngineType})
		models, err := repo.ListProviderModels(ctx, row.ID)
		if err != nil {
			return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
		}
		for _, model := range models {
			resolved, resolveErr := s.resolveMappedProviderModel(ctx, model)
			if resolveErr != nil || resolved.Model.LifecycleStatus != officialmodel.LifecycleActive {
				continue
			}
			label := strings.TrimSpace(model.DisplayName)
			if label == "" {
				label = model.ModelID
			}
			option := ModelOption{Label: label, Value: model.ModelID, ProviderID: row.ID, ModelID: model.ModelID, ModelKind: model.ModelKind, DisplayName: model.DisplayName, BillingMultiplier: "1"}
			effective, capabilityErr := s.effectiveCapabilityDTO(row.EngineType, resolved.Model.Capabilities, row.APIProtocol, true, attachmentPolicy)
			if capabilityErr != nil {
				continue
			}
			applyModelPriceToOption(&option, resolved, effective)
			modelOptions = append(modelOptions, option)
		}
	}
	return &InitResponse{Dict: InitDict{SceneArr: sceneOptions(), CommonStatusArr: dict.CommonStatusOptions(), ProviderOptions: options, ModelOptions: modelOptions, BillingMultiplierDefault: "1"}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	if query.Scene != "" && !isScene(query.Scene) {
		return nil, apperror.BadRequest("无效的智能体场景")
	}
	attachmentPolicy := s.resolveRequestAttachmentPolicy(ctx)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	list := make([]AgentDTO, 0, len(rows))
	for _, row := range rows {
		dto, priceErr := s.agentDTO(ctx, row, attachmentPolicy)
		if priceErr != nil {
			return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询当前模型价格失败", priceErr)
		}
		list = append(list, dto)
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) ProviderModels(ctx context.Context, providerID uint64) (*ProviderModelsResponse, *apperror.Error) {
	if providerID == 0 {
		return nil, apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := s.ensureActiveProvider(ctx, repo, providerID); appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListProviderModels(ctx, providerID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	list := make([]ProviderModelDTO, 0, len(rows))
	for _, row := range rows {
		resolved, resolveErr := s.resolveMappedProviderModel(ctx, row)
		if resolveErr != nil || resolved.Model.LifecycleStatus != officialmodel.LifecycleActive {
			continue
		}
		list = append(list, providerModelDTO(row))
	}
	return &ProviderModelsResponse{List: list}, nil
}

func (s *Service) Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	attachmentPolicy := s.resolveRequestAttachmentPolicy(ctx)
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI智能体不存在")
	}
	dto, priceErr := s.agentDTO(ctx, *row, attachmentPolicy)
	if priceErr != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询当前模型价格失败", priceErr)
	}
	return &DetailResponse{AgentDTO: dto}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	if appErr := s.ensureActiveProvider(ctx, repo, row.ProviderID); appErr != nil {
		return 0, appErr
	}
	requiredKind, kindErr := requiredModelKind(decodeScenes(row.ScenesJSON))
	if kindErr != nil {
		return 0, agentModelSceneMismatch(kindErr)
	}
	model, appErr := s.ensureProviderModel(ctx, repo, row.ProviderID, row.ModelID, requiredKind)
	if appErr != nil {
		return 0, appErr
	}
	row.ProviderModelID = model.ID
	row.ModelDisplayName = model.DisplayName
	if appErr := s.ensureOfficialModelSelectable(ctx, *model); appErr != nil {
		return 0, appErr
	}
	if appErr := s.requireAssignableContextProfile(ctx, row.ContextProfileID); appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "新增AI智能体失败", err)
	}
	if row.ContextProfileID != nil {
		s.notifyContextProfileAssignmentCommitted(ctx, id, *row.ContextProfileID)
	}
	return id, nil
}

func (s *Service) notifyContextProfileAssignmentCommitted(ctx context.Context, agentID uint64, profileID uint64) {
	if s == nil || s.contextProfiles == nil {
		return
	}
	committer, ok := s.contextProfiles.(contextProfileAssignmentCommitter)
	if !ok {
		return
	}
	_ = committer.ContextProfileAssignmentCommitted(ctx, agentID, profileID)
}

func (s *Service) Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if strings.TrimSpace(input.BillingMultiplier) == "" {
		input.BillingMultiplier = formatMultiplier(defaultMultiplier(row.BillingMultiplierPPM))
	}
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return appErr
	}
	if appErr := s.ensureActiveProvider(ctx, repo, input.ProviderID); appErr != nil {
		return appErr
	}
	model, appErr := s.ensureProviderModel(ctx, repo, input.ProviderID, fields.modelID, fields.requiredModelKind)
	if appErr != nil {
		return appErr
	}
	fields.providerModelID = model.ID
	fields.modelDisplayName = model.DisplayName
	if row.ProviderID != fields.providerID || strings.TrimSpace(row.ModelID) != fields.modelID {
		if appErr := s.ensureOfficialModelSelectable(ctx, *model); appErr != nil {
			return appErr
		}
	} else if appErr := s.ensureOfficialModelCallable(ctx, *model); appErr != nil {
		return appErr
	}
	if !sameOptionalUint64(row.ContextProfileID, fields.contextProfileID) {
		if appErr := s.requireContextProfileChange(ctx, id, fields.contextProfileID); appErr != nil {
			return appErr
		}
	}
	updateFields := updateFieldsMap(fields)
	if err := repo.Update(ctx, id, updateFields); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "编辑AI智能体失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "切换AI智能体状态失败", err)
	}
	return nil
}

func (s *Service) Test(ctx context.Context, id uint64) (*infraai.TestConnectionResult, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI智能体不存在")
	}
	if row.Status != enum.CommonYes {
		return nil, apperror.BadRequest("AI智能体已禁用")
	}
	requiredKind, kindErr := requiredModelKind(decodeScenes(row.ScenesJSON))
	if kindErr != nil {
		return nil, agentModelSceneMismatch(kindErr)
	}
	model, appErr := s.ensureProviderModel(ctx, repo, row.ProviderID, row.ModelID, requiredKind)
	if appErr != nil {
		return nil, appErr
	}
	if appErr := s.ensureOfficialModelCallable(ctx, *model); appErr != nil {
		return nil, appErr
	}
	connection, err := repo.GetActiveProvider(ctx, row.ProviderID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return nil, apperror.BadRequest("AI供应商不存在或已禁用")
	}
	apiKeyEnc := strings.TrimSpace(connection.APIKeyEnc)
	if apiKeyEnc == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	tester := s.tester
	if tester == nil {
		tester = unsupportedTester{}
	}
	result, testErr := tester.TestConnection(ctx, infraai.TestConnectionInput{EngineType: infraai.EngineType(connection.EngineType), BaseURL: connection.BaseURL, APIKey: apiKey, TimeoutMs: 10000})
	if testErr != nil {
		return result, apperror.LegacyWrap(apperror.CodeInternal, 500, "测试AI智能体失败", testErr)
	}
	return result, nil
}

func (s *Service) Delete(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "删除AI智能体失败", err)
	}
	return nil
}

func (s *Service) Options(ctx context.Context, query OptionQuery) (*AgentOptionsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Scene = strings.TrimSpace(query.Scene)
	if query.Scene == "" {
		query.Scene = sceneChat
	}
	if !isScene(query.Scene) {
		return nil, apperror.BadRequest("无效的智能体场景")
	}
	attachmentPolicy := s.resolveRequestAttachmentPolicy(ctx)
	rows, err := repo.ListVisibleAgents(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询可用AI智能体失败", err)
	}
	list := make([]AgentOption, 0, len(rows))
	for _, row := range rows {
		if row.Status != enum.CommonYes || row.IsDel == enum.CommonYes {
			continue
		}
		model, err := s.resolveModelPrice(ctx, row.ModelID)
		if err != nil || model.Model.LifecycleStatus == officialmodel.LifecycleRetired {
			continue
		}
		effective, capabilityErr := s.effectiveCapabilityDTO(row.EngineType, model.Model.Capabilities, row.APIProtocol, row.providerRouteEnabled(), attachmentPolicy)
		if capabilityErr != nil {
			continue
		}
		list = append(list, AgentOption{
			ID: row.ID, Name: row.Name, Avatar: row.Avatar, SystemPrompt: row.SystemPrompt,
			ProviderModelID: row.ProviderModelID, OfficialModel: officialModelSummary(model.Model),
			Capabilities: effective,
		})
	}
	return &AgentOptionsResponse{List: list}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI智能体仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) ensureActiveProvider(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	connection, err := repo.GetActiveProvider(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return apperror.BadRequest("AI供应商不存在或已禁用")
	}
	return nil
}

func (s *Service) requireAssignableContextProfile(ctx context.Context, profileID *uint64) *apperror.Error {
	if profileID == nil {
		return nil
	}
	if *profileID == 0 {
		return apperror.BadRequest("无效的上下文配置ID")
	}
	if s == nil || s.contextProfiles == nil {
		return contextProfileUnavailable(nil)
	}
	if err := s.contextProfiles.RequireAssignable(ctx, *profileID); err != nil {
		return contextProfileResolverError(err)
	}
	return nil
}

func (s *Service) requireContextProfileChange(ctx context.Context, agentID uint64, profileID *uint64) *apperror.Error {
	if s == nil || s.contextProfiles == nil {
		return contextProfileUnavailable(nil)
	}
	if err := s.contextProfiles.RequireAgentProfileChangeAllowed(ctx, agentID, profileID); err != nil {
		return contextProfileResolverError(err)
	}
	return s.requireAssignableContextProfile(ctx, profileID)
}

func contextProfileResolverError(err error) *apperror.Error {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return contextProfileUnavailable(err)
}

func contextProfileUnavailable(cause error) *apperror.Error {
	if cause != nil {
		return apperror.Wrap("ai.context.profile_unavailable", apperror.CategoryDependency, 503, apperror.Permanent, "ai.context.profile_unavailable", nil, "上下文配置当前不可用", cause)
	}
	return apperror.New("ai.context.profile_unavailable", apperror.CategoryDependency, 503, apperror.Permanent, "ai.context.profile_unavailable", nil, "上下文配置当前不可用")
}

func sameOptionalUint64(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *Service) ensureProviderModel(ctx context.Context, repo Repository, providerID uint64, modelID string, requiredKind aiprovider.ModelKind) (*ProviderModel, *apperror.Error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, apperror.BadRequest("关联模型不能为空")
	}
	models, err := repo.ListProviderModels(ctx, providerID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	matchedID := false
	for _, model := range models {
		if strings.TrimSpace(model.ModelID) != modelID {
			continue
		}
		matchedID = true
		if model.Status == enum.CommonYes && model.ModelKind == requiredKind {
			if requiredKind == aiprovider.ModelKindImage && modelID != supportedImageModelID {
				return nil, agentModelSceneMismatch(errors.New("image model adapter is not implemented"))
			}
			if model.MappingStatus != officialmodel.MappingStatusMapped || model.OfficialModelID == nil || model.OfficialCatalogVersion == nil || model.MappedAt == nil {
				return nil, apperror.Wrap("ai.official_model.unmapped", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该供应商模型未映射到官方模型", nil)
			}
			return &model, nil
		}
	}
	if matchedID {
		return nil, agentModelSceneMismatch(errors.New("provider model kind does not match agent scenes"))
	}
	return nil, apperror.BadRequest("关联模型不存在或已禁用")
}

func agentModelSceneMismatch(cause error) *apperror.Error {
	return apperror.Wrap("ai.agent.model_scene_mismatch", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "智能体场景与模型用途不匹配", cause)
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
	query.Name = strings.TrimSpace(query.Name)
	query.Scene = strings.TrimSpace(query.Scene)
	return query
}

func normalizeCreateInput(input CreateInput) (Agent, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return Agent{}, appErr
	}
	return Agent{
		ProviderID:           fields.providerID,
		Name:                 fields.name,
		ModelID:              fields.modelID,
		ScenesJSON:           fields.scenesJSON,
		SystemPrompt:         fields.systemPrompt,
		Avatar:               fields.avatar,
		Status:               fields.status,
		IsDel:                enum.CommonNo,
		BillingMultiplierPPM: fields.billingMultiplierPPM,
		ContextProfileID:     fields.contextProfileID,
	}, nil
}

func updateFieldsMap(fields normalizedFields) map[string]any {
	out := map[string]any{
		"provider_id":            fields.providerID,
		"provider_model_id":      fields.providerModelID,
		"name":                   fields.name,
		"model_id":               fields.modelID,
		"scenes_json":            fields.scenesJSON,
		"system_prompt":          fields.systemPrompt,
		"avatar":                 fields.avatar,
		"status":                 fields.status,
		"billing_multiplier_ppm": fields.billingMultiplierPPM,
	}
	if fields.contextProfileID == nil {
		out["context_profile_id"] = nil
	} else {
		out["context_profile_id"] = *fields.contextProfileID
	}
	if fields.modelDisplayName != "" {
		out["model_display_name"] = fields.modelDisplayName
	}
	return out
}

type normalizedFields struct {
	providerID           uint64
	providerModelID      uint64
	name                 string
	modelID              string
	modelDisplayName     string
	scenesJSON           string
	systemPrompt         string
	avatar               string
	status               int
	billingMultiplierPPM int64
	contextProfileID     *uint64
	requiredModelKind    aiprovider.ModelKind
}

func normalizeMutationFields(input CreateInput) (normalizedFields, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	modelID := strings.TrimSpace(input.ModelID)
	systemPrompt := strings.TrimSpace(input.SystemPrompt)
	avatar := strings.TrimSpace(input.Avatar)
	if input.ProviderID == 0 {
		return normalizedFields{}, apperror.BadRequest("AI供应商不能为空")
	}
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("AI智能体名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI智能体名称不能超过128个字符")
	}
	if modelID == "" {
		return normalizedFields{}, apperror.BadRequest("关联模型不能为空")
	}
	if len([]rune(modelID)) > 191 {
		return normalizedFields{}, apperror.BadRequest("关联模型不能超过191个字符")
	}
	scenesJSON, appErr := encodeScenes(input.Scenes)
	if appErr != nil {
		return normalizedFields{}, appErr
	}
	requiredKind, err := requiredModelKind(decodeScenes(scenesJSON))
	if err != nil {
		return normalizedFields{}, agentModelSceneMismatch(err)
	}
	if len([]rune(systemPrompt)) > 20000 {
		return normalizedFields{}, apperror.BadRequest("系统提示词不能超过20000个字符")
	}
	if len([]rune(avatar)) > 512 {
		return normalizedFields{}, apperror.BadRequest("头像地址不能超过512个字符")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	multiplier, err := parseBillingMultiplier(input.BillingMultiplier)
	if err != nil {
		return normalizedFields{}, apperror.BadRequest("billing_multiplier必须是大于0且最多6位小数的十进制数")
	}
	return normalizedFields{providerID: input.ProviderID, name: name, modelID: modelID, scenesJSON: scenesJSON, systemPrompt: systemPrompt, avatar: avatar, status: status, billingMultiplierPPM: multiplier, contextProfileID: input.ContextProfileID, requiredModelKind: requiredKind}, nil
}

func (s *Service) agentDTO(ctx context.Context, row AgentWithProvider, attachmentPolicy requestAttachmentPolicy) (AgentDTO, error) {
	scenes := decodeScenes(row.ScenesJSON)
	multiplier := defaultMultiplier(row.BillingMultiplierPPM)
	dto := AgentDTO{ID: row.ID, ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType, Name: row.Name, ModelID: row.ModelID, ModelKind: row.ProviderModelKind, ModelDisplayName: row.ModelDisplayName, Scenes: scenes, SceneNames: sceneNames(scenes), SystemPrompt: row.SystemPrompt, Avatar: row.Avatar, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt), BillingMultiplier: formatMultiplier(multiplier), ContextProfileID: row.ContextProfileID}
	model, err := s.resolveModelPrice(ctx, row.ModelID)
	if err == nil {
		effective, capabilityErr := s.effectiveCapabilityDTO(row.EngineType, model.Model.Capabilities, row.APIProtocol, row.providerRouteEnabled(), attachmentPolicy)
		if capabilityErr != nil {
			return AgentDTO{}, capabilityErr
		}
		dto.ProviderModelID = row.ProviderModelID
		applyModelPriceToAgent(&dto, model, effective)
	} else if !errors.Is(err, pricing.ErrPriceUnavailable) {
		return AgentDTO{}, err
	}
	return dto, nil
}

func applyModelPriceToOption(option *ModelOption, model officialmodel.ResolvedModel, effective *EffectiveCapabilitiesDTO) {
	option.OfficialModel = officialModelSummary(model.Model)
	option.Capabilities = effective
	option.PricingVersion, option.CatalogVersion = model.PricingVersion(), model.Model.CatalogVersion
	option.CatalogVendor, option.CatalogModelID = model.Model.CatalogVendor, model.Model.ModelID
	option.PriceSource, option.OverrideVersion = model.PriceSource, model.OverrideVersion
	option.PriceSourceURL, option.PriceVerifiedAt = model.PriceSourceURL, model.PriceVerifiedAt.UTC().Format(time.DateOnly)
	option.ContextTierThresholdTokens, option.CatalogRates = model.Model.ContextTierThresholdTokens, catalogRates(model.EffectivePrice)
}

func applyModelPriceToAgent(dto *AgentDTO, model officialmodel.ResolvedModel, effective *EffectiveCapabilitiesDTO) {
	dto.OfficialModel = officialModelSummary(model.Model)
	dto.Capabilities = effective
	dto.PricingVersion, dto.CatalogVersion = model.PricingVersion(), model.Model.CatalogVersion
	dto.CatalogVendor, dto.CatalogModelID = model.Model.CatalogVendor, model.Model.ModelID
	dto.PriceSource, dto.OverrideVersion = model.PriceSource, model.OverrideVersion
	dto.PriceSourceURL, dto.PriceVerifiedAt = model.PriceSourceURL, model.PriceVerifiedAt.UTC().Format(time.DateOnly)
	dto.ContextTierThresholdTokens, dto.CatalogRates = model.Model.ContextTierThresholdTokens, catalogRates(model.EffectivePrice)
}

func officialModelSummary(model officialmodel.Model) *OfficialModelSummaryDTO {
	return &OfficialModelSummaryDTO{
		ModelID: model.ModelID, ModelKind: model.ModelKind, CatalogVersion: model.CatalogVersion, CatalogVendor: model.CatalogVendor, ModelFamily: model.ModelFamily,
		LifecycleStatus: model.LifecycleStatus, ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
	}
}

func (s *Service) effectiveCapabilityDTO(
	engineType string,
	official officialmodel.Capabilities,
	providerProtocol string,
	routeEnabled bool,
	attachmentPolicy requestAttachmentPolicy,
) (*EffectiveCapabilitiesDTO, error) {
	if s == nil || s.capabilities == nil {
		return nil, capability.ErrTransportCapabilitiesUnavailable
	}
	metadata, ok := s.capabilities.ResolveCapabilities(infraai.EngineType(strings.TrimSpace(engineType)))
	if !ok {
		return nil, capability.ErrTransportCapabilitiesUnavailable
	}
	effective, err := capability.EffectiveChatCapabilities(official, metadata, routeEnabled)
	if err != nil {
		return nil, err
	}
	nativeFile := capability.ResolveNativeFileCapability(capability.NativeFileCapabilityInput{
		OfficialEnabled:      official.NativeFileInput && containsString(official.InputModalities, officialmodel.ModalityFile),
		TransportEnabled:     containsString(metadata.InputModalities, officialmodel.ModalityFile),
		ProviderProtocol:     providerProtocol,
		ProviderRouteEnabled: routeEnabled,
		PlatformReady:        attachmentPolicy.PlatformReady,
		AcceptedExtensions:   attachmentPolicy.AcceptedExtensions,
	})
	if !nativeFile.Enabled {
		effective.InputModalities = withoutString(effective.InputModalities, officialmodel.ModalityFile)
		effective.NativeFileInput = false
	}
	return newEffectiveCapabilityDTO(effective, nativeFile), nil
}

func newEffectiveCapabilityDTO(value officialmodel.Capabilities, nativeFile capability.NativeFileCapability) *EffectiveCapabilitiesDTO {
	image := ImageAttachmentCapability{MIMETypes: []string{}}
	if value.ImageInput != nil && containsString(value.InputModalities, officialmodel.ModalityImage) {
		image = ImageAttachmentCapability{
			Enabled: true, MIMETypes: append([]string(nil), value.ImageInput.MIMETypes...),
			MaxFiles: value.ImageInput.MaxFiles, MaxFileBytes: value.ImageInput.MaxBytes,
		}
	}
	return &EffectiveCapabilitiesDTO{
		InputModalities: append([]string(nil), value.InputModalities...), OutputModalities: append([]string(nil), value.OutputModalities...),
		SupportsTools: value.SupportsTools, SupportsStreaming: value.SupportsStreaming,
		SupportsStructuredOutput: value.SupportsStructuredOutput,
		RuntimeParameters: RuntimeParameterCapabilities{
			Temperature: TemperatureParameterCapability{
				Supported: containsString(value.SupportedParameters, officialmodel.ParameterTemperature), Default: 1, Min: 0, Max: 2,
			},
		},
		Attachments: AttachmentCapabilities{
			MaxAttachmentsPerMessage:  capability.MaxAttachmentsPerMessage,
			MaxMessageAttachmentBytes: capability.MaxMessageAttachmentBytes,
			Image:                     image,
			NativeFile: NativeFileAttachmentCapability{
				Enabled: nativeFile.Enabled, DisabledReason: nativeFile.DisabledReason,
				MaxFilesPerMessage: capability.MaxAttachmentsPerMessage, MaxFileBytesExclusive: capability.MaxNativeFileBytesExclusive,
				MaxRequestFileBytes: capability.MaxRequestNativeFileBytes, AcceptedExtensions: append([]string{}, nativeFile.AcceptedExtensions...),
			},
		},
	}
}

type requestAttachmentPolicy struct {
	PlatformReady      bool
	AcceptedExtensions []string
}

func (s *Service) resolveRequestAttachmentPolicy(ctx context.Context) requestAttachmentPolicy {
	if s == nil || s.uploadRules == nil {
		return requestAttachmentPolicy{}
	}
	rule, err := s.uploadRules.ResolveActive(ctx)
	if err != nil {
		return requestAttachmentPolicy{}
	}
	accepted := capability.AllowedNativeFileExtensions(rule.FileExtensions)
	return requestAttachmentPolicy{
		PlatformReady:      len(accepted) > 0,
		AcceptedExtensions: accepted,
	}
}

func withoutString(values []string, unwanted string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != unwanted {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (row AgentWithProvider) providerRouteEnabled() bool {
	return row.ProviderStatus == enum.CommonYes && row.ProviderModelStatus == enum.CommonYes &&
		row.ProviderModelID > 0 && row.MappingStatus == officialmodel.MappingStatusMapped &&
		strings.TrimSpace(row.OfficialModelID) != "" && strings.TrimSpace(row.OfficialCatalogVersion) != ""
}

func catalogRates(model pricing.PriceBook) []CatalogRateDTO {
	rates := make([]CatalogRateDTO, 0, len(model.Rates))
	for _, rate := range model.Rates {
		formatted, formatErr := sharedmoney.FormatRMBUnits(rate.PriceUnits)
		if formatErr == nil {
			rates = append(rates, CatalogRateDTO{Category: string(rate.Category), Unit: rate.Unit, TierKey: rate.TierKey, Price: formatted, UnitScale: rate.UnitScale})
		}
	}
	return rates
}

func (s *Service) ensureOfficialModelSelectable(ctx context.Context, route ProviderModel) *apperror.Error {
	model, err := s.resolveMappedProviderModel(ctx, route)
	if err != nil {
		if errors.Is(err, officialmodel.ErrModelRetired) {
			return apperror.Wrap("ai.official_model.not_selectable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该官方模型不允许新建或切换智能体", nil)
		}
		return apperror.Wrap("ai.billing.price_unavailable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该智能体缺少可用的模型价格", err)
	}
	if model.Model.LifecycleStatus != officialmodel.LifecycleActive {
		return apperror.Wrap("ai.official_model.not_selectable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该官方模型不允许新建或切换智能体", nil)
	}
	return nil
}

func (s *Service) ensureOfficialModelCallable(ctx context.Context, route ProviderModel) *apperror.Error {
	model, err := s.resolveMappedProviderModel(ctx, route)
	if err != nil {
		if errors.Is(err, officialmodel.ErrModelRetired) {
			return apperror.Wrap("ai.official_model.retired", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该官方模型已退役", nil)
		}
		return apperror.Wrap("ai.billing.price_unavailable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该智能体缺少可用的模型价格", err)
	}
	if model.Model.LifecycleStatus == officialmodel.LifecycleRetired {
		return apperror.Wrap("ai.official_model.retired", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该官方模型已退役", nil)
	}
	return nil
}

func (s *Service) resolveMappedProviderModel(ctx context.Context, route ProviderModel) (officialmodel.ResolvedModel, error) {
	if route.Status != enum.CommonYes || route.MappingStatus != officialmodel.MappingStatusMapped || route.OfficialModelID == nil ||
		route.OfficialCatalogVersion == nil || route.MappedAt == nil {
		return officialmodel.ResolvedModel{}, officialmodel.ErrModelUnmapped
	}
	resolved, err := officialmodel.ResolveMappedRoute(
		ctx,
		s.pricingResolver,
		route.ModelID,
		strings.TrimSpace(*route.OfficialModelID),
		strings.TrimSpace(*route.OfficialCatalogVersion),
		route.MappingStatus,
	)
	if err != nil {
		return officialmodel.ResolvedModel{}, err
	}
	if resolved.Model.ModelKind != route.ModelKind {
		return officialmodel.ResolvedModel{}, officialmodel.ErrModelMappingStale
	}
	return resolved, nil
}

func (s *Service) resolveModelPrice(ctx context.Context, modelID string) (officialmodel.ResolvedModel, error) {
	if s == nil || s.pricingResolver == nil {
		return officialmodel.ResolvedModel{}, officialmodel.ErrRepositoryNotConfigured
	}
	return s.pricingResolver.Resolve(ctx, strings.TrimSpace(modelID))
}

func defaultMultiplier(value int64) int64 {
	if value <= 0 {
		return 1000000
	}
	return value
}

func parseBillingMultiplier(input string) (int64, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 1000000, nil
	}
	if strings.HasPrefix(input, "+") {
		input = input[1:]
	}
	if strings.HasPrefix(input, "-") || input == "" {
		return 0, pricing.ErrInvalidMultiplier
	}
	parts := strings.Split(input, ".")
	if len(parts) > 2 || parts[0] == "" || !allDigits(parts[0]) {
		return 0, pricing.ErrInvalidMultiplier
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if len(frac) > 6 || !allDigits(frac) {
			return 0, pricing.ErrInvalidMultiplier
		}
	}
	frac += strings.Repeat("0", 6-len(frac))
	integer, ok := new(big.Int).SetString(parts[0], 10)
	if !ok {
		return 0, pricing.ErrInvalidMultiplier
	}
	value := new(big.Int).Mul(integer, big.NewInt(1000000))
	if frac != "" {
		f, ok := new(big.Int).SetString(frac, 10)
		if !ok {
			return 0, pricing.ErrInvalidMultiplier
		}
		value.Add(value, f)
	}
	if value.Sign() <= 0 || !value.IsInt64() {
		return 0, pricing.ErrInvalidMultiplier
	}
	return value.Int64(), nil
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func formatMultiplier(ppm int64) string {
	if ppm%1000000 == 0 {
		return strconv.FormatInt(ppm/1000000, 10)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%d.%06d", ppm/1000000, ppm%1000000), "0"), ".")
}

func providerModelDTO(row ProviderModel) ProviderModelDTO {
	return ProviderModelDTO{
		ID: row.ID, ProviderID: row.ProviderID, ModelID: row.ModelID, ModelKind: row.ModelKind, DisplayName: row.DisplayName,
		OfficialModelID: pointerValue(row.OfficialModelID), OfficialCatalogVersion: pointerValue(row.OfficialCatalogVersion),
		MappingStatus: row.MappingStatus, MappedAt: formatOptionalTime(row.MappedAt),
		Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func sceneOptions() []dict.Option[string] {
	return stringOptions([]string{sceneChat, sceneAgentGenerate, capability.SceneTextGenerate, capability.SceneImageGenerate}, sceneLabels)
}
func stringOptions(values []string, labels map[string]string) []dict.Option[string] {
	options := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, dict.Option[string]{Label: labels[value], Value: value})
	}
	return options
}

func isScene(value string) bool { _, ok := sceneLabels[value]; return ok }

func requiredModelKind(scenes []string) (aiprovider.ModelKind, error) {
	if len(scenes) == 0 {
		return aiprovider.ModelKindChat, nil
	}
	for _, scene := range scenes {
		if scene == capability.SceneImageGenerate {
			if len(scenes) != 1 {
				return "", errors.New("image_generate must be the only scene")
			}
			return aiprovider.ModelKindImage, nil
		}
	}
	return aiprovider.ModelKindChat, nil
}

func encodeScenes(values []string) (string, *apperror.Error) {
	if len(values) == 0 {
		values = []string{sceneChat}
	}
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		scene := strings.TrimSpace(value)
		if !isScene(scene) {
			return "", apperror.BadRequest("无效的智能体场景")
		}
		if _, ok := seen[scene]; ok {
			continue
		}
		seen[scene] = struct{}{}
		normalized = append(normalized, scene)
	}
	if len(normalized) == 0 {
		normalized = []string{sceneChat}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", apperror.BadRequest("智能体场景不是合法JSON")
	}
	return string(data), nil
}

func decodeScenes(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "null" {
		return []string{sceneChat}
	}
	var scenes []string
	if err := json.Unmarshal([]byte(value), &scenes); err != nil || len(scenes) == 0 {
		return []string{sceneChat}
	}
	out := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		scene = strings.TrimSpace(scene)
		if isScene(scene) {
			out = append(out, scene)
		}
	}
	if len(out) == 0 {
		return []string{sceneChat}
	}
	return out
}

func sceneNames(scenes []string) []string {
	names := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		if label := sceneLabels[scene]; label != "" {
			names = append(names, label)
		}
	}
	return names
}

func statusText(value int) string {
	for _, item := range dict.CommonStatusOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return ""
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

type unsupportedTester struct{}

func (unsupportedTester) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	return nil, fmt.Errorf("ai agent tester not configured")
}
