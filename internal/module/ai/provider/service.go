package aiprovider

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/provider"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
)

const (
	timeLayout           = "2006-01-02 15:04:05"
	driverOpenAI         = "openai"
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	maxStoredErrorRunes  = 1024
)

var engineTypeLabels = map[string]string{
	driverOpenAI: "OpenAI",
}

var apiProtocolLabels = map[string]string{
	APIProtocolChatCompletions: "Chat Completions",
	APIProtocolResponses:       "Responses API",
}

var healthStatusOptions = []dict.Option[string]{
	{Label: "未知", Value: provider.HealthUnknown},
	{Label: "正常", Value: provider.HealthOK},
	{Label: "失败", Value: provider.HealthFailed},
}

var modelSyncStatusOptions = healthStatusOptions

type Service struct {
	repository Repository
	secretbox  secretbox.Box
	tester     ProviderTester
	driver     ModelDriver
	matcher    officialmodel.IdentityMatcher
	now        func() time.Time
}

type Option func(*Service)

func WithOfficialModelMatcher(matcher officialmodel.IdentityMatcher) Option {
	return func(service *Service) {
		if matcher != nil {
			service.matcher = matcher
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func NewService(repository Repository, box secretbox.Box, tester ProviderTester, options ...Option) *Service {
	service := &Service{
		repository: repository, secretbox: box, tester: tester, driver: provider.NewOpenAIDriver(nil),
		matcher: officialmodel.NewIdentityMatcher(officialmodel.Default), now: time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func NewServiceWithDriver(repository Repository, box secretbox.Box, tester ProviderTester, driver ModelDriver, options ...Option) *Service {
	service := NewService(repository, box, tester, options...)
	if driver != nil {
		service.driver = driver
	}
	return service
}

func (s *Service) PageInit(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		EngineTypeArr: engineTypeOptions(), APIProtocolArr: apiProtocolOptions(),
		CommonStatusArr: dict.CommonStatusOptions(), HealthStatusArr: healthStatusOptions, ModelSyncArr: modelSyncStatusOptions,
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	list := make([]ProviderDTO, 0, len(rows))
	for _, row := range rows {
		models, err := repo.ListModels(ctx, row.ID)
		if err != nil {
			return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
		}
		dto, appErr := providerDTO(row, models)
		if appErr != nil {
			return nil, appErr
		}
		list = append(list, dto)
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return 0, apperror.BadRequest("API Key不能为空")
	}
	row, catalog, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	catalog.models, appErr = s.mapProviderModels(catalog.models)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByTypeName(ctx, row.EngineType, row.Name, 0)
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "校验AI供应商失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("该驱动下已存在同名供应商")
	}
	ciphertext, err := s.secretbox.Encrypt(apiKey)
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "加密AI供应商API Key失败", err)
	}
	row.APIKeyEnc = ciphertext
	row.APIKeyHint = secretbox.Hint(apiKey)
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "新增AI供应商失败", err)
	}
	if err := repo.ReconcileModels(ctx, id, catalog.scope, catalog.models); err != nil {
		return 0, providerModelWriteError(err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	fields, catalog, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	catalog.models, appErr = s.mapProviderModels(catalog.models)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByTypeName(ctx, strings.TrimSpace(input.EngineType), strings.TrimSpace(input.Name), id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "校验AI供应商失败", err)
	}
	if exists {
		return apperror.BadRequest("该驱动下已存在同名供应商")
	}
	if strings.TrimSpace(input.APIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return apperror.LegacyWrap(apperror.CodeInternal, 500, "加密AI供应商API Key失败", err)
		}
		fields["api_key_enc"] = ciphertext
		fields["api_key_hint"] = secretbox.Hint(strings.TrimSpace(input.APIKey))
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "编辑AI供应商失败", err)
	}
	if err := repo.ReconcileModels(ctx, id, catalog.scope, catalog.models); err != nil {
		return providerModelWriteError(err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI供应商ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "切换AI供应商状态失败", err)
	}
	return nil
}

func (s *Service) TestConnection(ctx context.Context, id uint64) (*infraai.TestConnectionResult, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI供应商不存在")
	}
	if row.Status != enum.CommonYes {
		return nil, apperror.BadRequest("AI供应商已禁用")
	}
	apiKey, err := s.secretbox.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	result, testErr := s.testOpenAI(ctx, normalizeProviderBaseURL(row.EngineType, row.BaseURL), apiKey)
	now := time.Now()
	health := provider.HealthOK
	message := ""
	if testErr != nil || result == nil || !result.OK {
		health = provider.HealthFailed
		message = truncateErrorString(errorMessage(testErr, result))
	}
	fields := map[string]any{"health_status": health, "last_checked_at": now, "last_check_error": message}
	if err := repo.Update(ctx, id, fields); err != nil {
		return result, apperror.LegacyWrap(apperror.CodeInternal, 500, "更新AI供应商健康状态失败", err)
	}
	if testErr != nil {
		return result, apperror.LegacyWrap(apperror.CodeInternal, 500, "测试AI供应商连接失败", testErr)
	}
	return result, nil
}

func (s *Service) PreviewModels(ctx context.Context, input ModelOptionsInput) (*ModelOptionsResponse, *apperror.Error) {
	engineType, appErr := requireEngineType(input.EngineType)
	if appErr != nil {
		return nil, appErr
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return nil, apperror.BadRequest("API Key不能为空")
	}
	models, err := s.openAIDriver().ListModels(ctx, provider.Config{Driver: engineType, BaseURL: normalizeProviderBaseURL(engineType, input.BaseURL), APIKey: apiKey, TimeoutMs: 10000})
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "拉取OpenAI模型失败", err)
	}
	return &ModelOptionsResponse{List: s.modelCandidates(models)}, nil
}

func (s *Service) PreviewStoredModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI供应商不存在")
	}
	apiKey, err := s.secretbox.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	models, listErr := s.openAIDriver().ListModels(ctx, provider.Config{Driver: row.EngineType, BaseURL: normalizeProviderBaseURL(row.EngineType, row.BaseURL), APIKey: apiKey, TimeoutMs: 10000})
	if listErr != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "拉取OpenAI模型失败", listErr)
	}
	return &ModelOptionsResponse{List: s.modelCandidates(models)}, nil
}

func (s *Service) SyncModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI供应商不存在")
	}
	apiKey, err := s.secretbox.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	models, listErr := s.openAIDriver().ListModels(ctx, provider.Config{Driver: row.EngineType, BaseURL: normalizeProviderBaseURL(row.EngineType, row.BaseURL), APIKey: apiKey, TimeoutMs: 10000})
	now := time.Now()
	fields := map[string]any{"last_model_sync_at": now}
	if listErr != nil {
		fields["last_model_sync_status"] = provider.HealthFailed
		fields["last_model_sync_error"] = truncateErrorString(listErr.Error())
		_ = repo.Update(ctx, id, fields)
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "同步OpenAI模型失败", listErr)
	}
	fields["last_model_sync_status"] = provider.HealthOK
	fields["last_model_sync_error"] = ""
	candidates := s.modelCandidates(models)
	merger, ok := repo.(discoveredModelMerger)
	if !ok {
		return nil, apperror.Internal("AI供应商模型同步仓储未配置")
	}
	trusted := providerModelsFromCandidates(candidates, now)
	if err := merger.MergeDiscoveredModels(ctx, id, trusted); err != nil {
		fields["last_model_sync_status"] = provider.HealthFailed
		fields["last_model_sync_error"] = truncateErrorString(err.Error())
		_ = repo.Update(ctx, id, fields)
		return nil, providerModelWriteError(err)
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "更新AI供应商模型同步状态失败", err)
	}
	return &ModelOptionsResponse{List: candidates}, nil
}

func (s *Service) ListProviderModels(ctx context.Context, id uint64) (*ProviderModelsResponse, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	models, err := repo.ListModels(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	list, appErr := providerModelDTOs(models)
	if appErr != nil {
		return nil, appErr
	}
	return &ProviderModelsResponse{List: list}, nil
}

func (s *Service) UpdateProviderModels(ctx context.Context, id uint64, input UpdateModelsInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	catalog, appErr := buildModelCatalog(input.ModelIDs, input.Models, input.ModelDisplayNames, input.Statuses)
	if appErr != nil {
		return appErr
	}
	catalog.models, appErr = s.mapProviderModels(catalog.models)
	if appErr != nil {
		return appErr
	}
	if err := repo.ReconcileModels(ctx, id, catalog.scope, catalog.models); err != nil {
		return providerModelWriteError(err)
	}
	return nil
}

func (s *Service) ReconcileOfficialModelMappings(ctx context.Context) error {
	if s == nil || s.repository == nil {
		return ErrRepositoryNotConfigured
	}
	rows, err := s.repository.ListAllModels(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	matcher := s.matcher
	if matcher == nil {
		matcher = officialmodel.NewIdentityMatcher(officialmodel.Default)
	}
	for _, row := range rows {
		mapping := matcher.MatchIdentity(row.ModelID, now)
		if mapping.Status == officialmodel.MappingStatusMapped && mapping.ModelKind != row.ModelKind {
			return ErrProviderModelKindConflict
		}
		if providerModelMappingEqual(row, mapping) {
			continue
		}
		if err := s.repository.UpdateModelMapping(ctx, row.ID, mapping); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "删除AI供应商失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI供应商仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) openAIDriver() ModelDriver {
	if s != nil && s.driver != nil {
		return s.driver
	}
	return provider.NewOpenAIDriver(nil)
}

func (s *Service) testOpenAI(ctx context.Context, baseURL string, apiKey string) (*infraai.TestConnectionResult, error) {
	result, err := s.openAIDriver().TestConnection(ctx, provider.Config{Driver: driverOpenAI, BaseURL: baseURL, APIKey: apiKey, TimeoutMs: 10000})
	if result == nil {
		return nil, err
	}
	return &infraai.TestConnectionResult{OK: result.OK, Status: result.Status, LatencyMs: int(result.LatencyMs), Message: result.Message}, err
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
	query.EngineType = strings.TrimSpace(query.EngineType)
	return query
}

func normalizeCreateInput(input CreateInput) (Provider, normalizedModelCatalog, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.EngineType, input.BaseURL, input.APIProtocol, input.Status)
	if appErr != nil {
		return Provider{}, normalizedModelCatalog{}, appErr
	}
	catalog, appErr := buildModelCatalog(input.ModelIDs, input.Models, input.ModelDisplayNames, input.Statuses)
	if appErr != nil {
		return Provider{}, normalizedModelCatalog{}, appErr
	}
	return Provider{Name: fields.name, EngineType: fields.engineType, BaseURL: fields.baseURL, APIProtocol: fields.apiProtocol, Status: fields.status, HealthStatus: provider.HealthUnknown, LastModelSyncStatus: provider.HealthUnknown, IsDel: enum.CommonNo}, catalog, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, normalizedModelCatalog, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.EngineType, input.BaseURL, input.APIProtocol, input.Status)
	if appErr != nil {
		return nil, normalizedModelCatalog{}, appErr
	}
	catalog, appErr := buildModelCatalog(input.ModelIDs, input.Models, input.ModelDisplayNames, input.Statuses)
	if appErr != nil {
		return nil, normalizedModelCatalog{}, appErr
	}
	return map[string]any{"name": fields.name, "engine_type": fields.engineType, "base_url": fields.baseURL, "api_protocol": fields.apiProtocol, "status": fields.status}, catalog, nil
}

type normalizedFields struct {
	name, engineType, baseURL, apiProtocol string
	status                                 int
}

func normalizeMutationFields(name, engineType, baseURL, apiProtocol string, status int) (normalizedFields, *apperror.Error) {
	name = strings.TrimSpace(name)
	var appErr *apperror.Error
	engineType, appErr = requireEngineType(engineType)
	if appErr != nil {
		return normalizedFields{}, appErr
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = normalizeProviderBaseURL(engineType, baseURL)
	apiProtocol = strings.TrimSpace(apiProtocol)
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("供应商名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("供应商名称不能超过128个字符")
	}
	if len([]rune(baseURL)) > 512 {
		return normalizedFields{}, apperror.BadRequest("供应商地址不能超过512个字符")
	}
	if !isAPIProtocol(apiProtocol) {
		return normalizedFields{}, apperror.BadRequest("无效的AI请求协议")
	}
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	return normalizedFields{name: name, engineType: engineType, baseURL: baseURL, apiProtocol: apiProtocol, status: status}, nil
}

func validateSelectedModels(modelIDs []string) ([]string, *apperror.Error) {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(modelIDs))
	for _, item := range modelIDs {
		modelID := strings.TrimSpace(item)
		if modelID == "" {
			continue
		}
		if !seen[modelID] {
			seen[modelID] = true
			normalized = append(normalized, modelID)
		}
	}
	if len(normalized) == 0 {
		return nil, apperror.BadRequest("请至少选择一个模型")
	}
	return normalized, nil
}

func buildProviderModels(modelIDs []string, displayNames map[string]string, statuses map[string]int) ([]ProviderModel, *apperror.Error) {
	normalizedIDs, appErr := validateSelectedModels(modelIDs)
	if appErr != nil {
		return nil, appErr
	}
	models := make([]ProviderModel, 0, len(normalizedIDs))
	for _, modelID := range normalizedIDs {
		status := enum.CommonYes
		if statuses != nil && statuses[modelID] != 0 {
			status = statuses[modelID]
		}
		if !enum.IsCommonStatus(status) {
			return nil, apperror.BadRequest("无效的模型状态")
		}
		models = append(models, ProviderModel{ModelID: modelID, ModelKind: ModelKindChat, DisplayName: strings.TrimSpace(displayNames[modelID]), Status: status})
	}
	return models, nil
}

type normalizedModelCatalog struct {
	scope  ModelReconcileScope
	models []ProviderModel
}

func buildModelCatalog(modelIDs []string, typed []ProviderModelInput, displayNames map[string]string, statuses map[string]int) (normalizedModelCatalog, *apperror.Error) {
	if len(modelIDs) > 0 && len(typed) > 0 {
		return normalizedModelCatalog{}, apperror.BadRequest("model_ids与models不能同时提交")
	}
	if len(typed) == 0 {
		models, appErr := buildProviderModels(modelIDs, displayNames, statuses)
		return normalizedModelCatalog{scope: ModelReconcileChatOnly, models: models}, appErr
	}
	legacyTyped := len(displayNames) > 0 || len(statuses) > 0
	models := make([]ProviderModel, 0, len(typed))
	seen := make(map[providerModelIdentity]struct{}, len(typed))
	seenLegacyIDs := make(map[string]struct{}, len(typed))
	for _, input := range typed {
		if legacyTyped && providerModelInputHasInlineMutation(input) {
			return normalizedModelCatalog{}, apperror.BadRequest("旧模型字段与完整模型行不能同时提交")
		}
		if !legacyTyped && input.Status == nil {
			return normalizedModelCatalog{}, apperror.BadRequest("AI模型状态不能为空")
		}
		if legacyTyped {
			modelID := strings.TrimSpace(input.ModelID)
			if _, duplicate := seenLegacyIDs[modelID]; duplicate {
				return normalizedModelCatalog{}, apperror.BadRequest("旧模型格式不能表达同一模型ID的多个用途")
			}
			seenLegacyIDs[modelID] = struct{}{}
			status := enum.CommonYes
			if statuses[modelID] != 0 {
				status = statuses[modelID]
			}
			input.DisplayName = stringPointer(strings.TrimSpace(displayNames[modelID]))
			input.Status = intPointer(status)
		}
		model, appErr := normalizeProviderModelInput(input, !legacyTyped)
		if appErr != nil {
			return normalizedModelCatalog{}, appErr
		}
		identity := providerModelIdentity{modelID: model.ModelID, kind: model.ModelKind}
		if _, exists := seen[identity]; exists {
			return normalizedModelCatalog{}, apperror.BadRequest("AI模型用途重复")
		}
		seen[identity] = struct{}{}
		models = append(models, model)
	}
	if len(models) == 0 {
		return normalizedModelCatalog{}, apperror.BadRequest("请至少选择一个模型")
	}
	return normalizedModelCatalog{scope: ModelReconcileAll, models: models}, nil
}

func providerModelInputHasInlineMutation(input ProviderModelInput) bool {
	return input.ID != nil || input.DisplayName != nil || input.Status != nil || input.EmbeddingDimensions != nil ||
		input.EmbeddingMaxInputTokens != nil || input.EmbeddingTokenCounterID != nil
}

func normalizeProviderModelInput(input ProviderModelInput, requireEmbeddingSpec bool) (ProviderModel, *apperror.Error) {
	model := ProviderModel{
		ModelID: strings.TrimSpace(input.ModelID), ModelKind: input.ModelKind,
		EmbeddingDimensions:     cloneUint32(input.EmbeddingDimensions),
		EmbeddingMaxInputTokens: cloneInt64(input.EmbeddingMaxInputTokens),
		EmbeddingTokenCounterID: trimOptionalString(input.EmbeddingTokenCounterID),
	}
	if input.ID != nil {
		model.ID = *input.ID
	}
	if input.DisplayName != nil {
		model.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Status == nil {
		return ProviderModel{}, apperror.BadRequest("AI模型状态不能为空")
	}
	model.Status = *input.Status
	if model.ModelKind == "" {
		return ProviderModel{}, providerValidationError("ai.provider.model_kind_confirmation_required", "请先确认AI模型用途")
	}
	if model.ModelID == "" {
		return ProviderModel{}, apperror.BadRequest("AI模型ID不能为空")
	}
	if err := model.ModelKind.Validate(); err != nil {
		return ProviderModel{}, providerValidationError("ai.provider.model_kind_invalid", "AI模型用途无效")
	}
	if !enum.IsCommonStatus(model.Status) {
		return ProviderModel{}, apperror.BadRequest("AI模型状态无效")
	}
	if err := validateEmbeddingSpec(model, requireEmbeddingSpec); err != nil {
		return ProviderModel{}, providerValidationError("ai.provider.embedding_spec_invalid", "向量模型规格不完整或无效")
	}
	return model, nil
}

func validateEmbeddingSpec(model ProviderModel, required bool) error {
	allNil := model.EmbeddingDimensions == nil && model.EmbeddingMaxInputTokens == nil && model.EmbeddingTokenCounterID == nil
	if model.ModelKind != ModelKindEmbedding {
		if !allNil {
			return errors.New("non-embedding model carries an embedding spec")
		}
		return nil
	}
	if allNil && (!required || model.ID > 0 && model.Status == enum.CommonNo) {
		return nil
	}
	if model.EmbeddingDimensions == nil || *model.EmbeddingDimensions == 0 || model.EmbeddingMaxInputTokens == nil || *model.EmbeddingMaxInputTokens <= 0 ||
		model.EmbeddingTokenCounterID == nil || strings.TrimSpace(*model.EmbeddingTokenCounterID) == "" {
		return errors.New("embedding model requires a complete spec")
	}
	_, err := infraai.ResolveTokenCounter(*model.EmbeddingTokenCounterID)
	return err
}

func cloneUint32(value *uint32) *uint32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringPointer(value string) *string { return &value }
func intPointer(value int) *int          { return &value }

func providerValidationError(code string, message string) *apperror.Error {
	return apperror.New(code, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "", nil, message)
}

func providerModelWriteError(err error) *apperror.Error {
	if errors.Is(err, ErrProviderModelInUse) || errors.Is(err, ErrProviderModelKindConflict) {
		code := "ai.provider.model_in_use"
		message := "模型已被智能体或上下文配置引用，不能修改用途或向量规格"
		if errors.Is(err, ErrProviderModelKindConflict) {
			code = "ai.provider.model_kind_conflict"
			message = "供应商模型用途与官方目录冲突"
		}
		return apperror.Wrap(code, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "", nil, message, err)
	}
	return apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "保存AI供应商模型失败", err)
}

func providerDTO(row Provider, models []ProviderModel) (ProviderDTO, *apperror.Error) {
	engineTypeName, ok := engineTypeLabels[row.EngineType]
	if !ok || !isAPIProtocol(row.APIProtocol) || !isHealthStatus(row.HealthStatus) || !isHealthStatus(row.LastModelSyncStatus) || !enum.IsCommonStatus(row.Status) {
		return ProviderDTO{}, apperror.InternalKey("aiprovider.data.invalid", nil, "AI供应商数据异常")
	}
	modelDTOs, appErr := providerModelDTOs(models)
	if appErr != nil {
		return ProviderDTO{}, appErr
	}
	enabledCount := 0
	for _, model := range models {
		if model.Status == enum.CommonYes {
			enabledCount++
		}
	}
	return ProviderDTO{
		ID:                  row.ID,
		Name:                row.Name,
		EngineType:          row.EngineType,
		EngineTypeName:      engineTypeName,
		BaseURL:             row.BaseURL,
		BaseURLEffective:    effectiveBaseURL(row.BaseURL),
		APIProtocol:         row.APIProtocol,
		APIKeyMasked:        row.APIKeyHint,
		HealthStatus:        row.HealthStatus,
		LastCheckedAt:       formatPtrTime(row.LastCheckedAt),
		LastCheckError:      row.LastCheckError,
		LastModelSyncAt:     formatPtrTime(row.LastModelSyncAt),
		LastModelSyncStatus: row.LastModelSyncStatus,
		LastModelSyncError:  row.LastModelSyncError,
		EnabledModelCount:   enabledCount,
		Models:              modelDTOs,
		Status:              row.Status,
		StatusName:          statusText(row.Status),
		CreatedAt:           formatTime(row.CreatedAt),
		UpdatedAt:           formatTime(row.UpdatedAt),
	}, nil
}

func providerModelDTOs(rows []ProviderModel) ([]ProviderModelDTO, *apperror.Error) {
	list := make([]ProviderModelDTO, 0, len(rows))
	for _, row := range rows {
		if !enum.IsCommonStatus(row.Status) || row.ModelKind.Validate() != nil || !validStoredMapping(row) {
			return nil, apperror.InternalKey("aiprovider.model.data_invalid", nil, "AI供应商模型数据异常")
		}
		list = append(list, ProviderModelDTO{
			ID: row.ID, ProviderID: row.ProviderID, ModelID: row.ModelID, ModelKind: row.ModelKind, DisplayName: row.DisplayName,
			OfficialModelID: valueOrEmpty(row.OfficialModelID), OfficialCatalogVersion: valueOrEmpty(row.OfficialCatalogVersion),
			MappingStatus: row.MappingStatus, MappedAt: formatPtrTime(row.MappedAt),
			EmbeddingDimensions: cloneUint32(row.EmbeddingDimensions), EmbeddingMaxInputTokens: cloneInt64(row.EmbeddingMaxInputTokens),
			EmbeddingTokenCounterID: cloneString(row.EmbeddingTokenCounterID),
			Status:                  row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
		})
	}
	return list, nil
}

func (s *Service) mapProviderModels(models []ProviderModel) ([]ProviderModel, *apperror.Error) {
	mapped := append([]ProviderModel(nil), models...)
	now := time.Now()
	if s != nil && s.now != nil {
		now = s.now()
	}
	matcher := officialmodel.IdentityMatcher(officialmodel.NewIdentityMatcher(officialmodel.Default))
	if s != nil && s.matcher != nil {
		matcher = s.matcher
	}
	for index := range mapped {
		mapping := matcher.MatchIdentity(mapped[index].ModelID, now)
		mapped[index].OfficialModelID = nil
		mapped[index].OfficialCatalogVersion = nil
		mapped[index].MappedAt = nil
		mapped[index].MappingStatus = mapping.Status
		if mapping.Status == officialmodel.MappingStatusMapped {
			if mapping.ModelKind != mapped[index].ModelKind {
				return nil, apperror.New("ai.provider.model_kind_conflict", apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "", nil, "供应商模型用途与官方目录冲突")
			}
			officialID, catalogVersion := mapping.OfficialModelID, mapping.CatalogVersion
			mapped[index].OfficialModelID = &officialID
			mapped[index].OfficialCatalogVersion = &catalogVersion
			mapped[index].MappedAt = mapping.MappedAt
			if mapping.EmbeddingSpec != nil {
				dimensions, maxInputTokens, tokenCounterID := mapping.EmbeddingSpec.Dimensions, mapping.EmbeddingSpec.MaxInputTokens, mapping.EmbeddingSpec.TokenCounterID
				mapped[index].EmbeddingDimensions = &dimensions
				mapped[index].EmbeddingMaxInputTokens = &maxInputTokens
				mapped[index].EmbeddingTokenCounterID = &tokenCounterID
			}
		}
	}
	return mapped, nil
}

func validStoredMapping(row ProviderModel) bool {
	switch row.MappingStatus {
	case officialmodel.MappingStatusMapped:
		return row.OfficialModelID != nil && strings.TrimSpace(*row.OfficialModelID) != "" &&
			row.OfficialCatalogVersion != nil && strings.TrimSpace(*row.OfficialCatalogVersion) != "" && row.MappedAt != nil
	case officialmodel.MappingStatusUnmapped:
		return row.OfficialModelID == nil && row.OfficialCatalogVersion == nil && row.MappedAt == nil
	default:
		return false
	}
}

func providerModelMappingEqual(row ProviderModel, mapping officialmodel.IdentityMapping) bool {
	if row.MappingStatus != mapping.Status {
		return false
	}
	if mapping.Status == officialmodel.MappingStatusUnmapped {
		return row.OfficialModelID == nil && row.OfficialCatalogVersion == nil && row.MappedAt == nil
	}
	return row.OfficialModelID != nil && *row.OfficialModelID == mapping.OfficialModelID &&
		row.OfficialCatalogVersion != nil && *row.OfficialCatalogVersion == mapping.CatalogVersion &&
		row.MappedAt != nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) modelCandidates(models []provider.Model) []ModelOptionDTO {
	matcher := officialmodel.IdentityMatcher(officialmodel.NewIdentityMatcher(officialmodel.Default))
	if s != nil && s.matcher != nil {
		matcher = s.matcher
	}
	now := time.Now()
	if s != nil && s.now != nil {
		now = s.now()
	}
	list := make([]ModelOptionDTO, 0, len(models))
	for _, model := range models {
		mapping := matcher.MatchIdentity(model.ID, now)
		candidate := ModelOptionDTO{ModelID: model.ID, DisplayName: model.ID, OwnedBy: model.OwnedBy, MappingStatus: mapping.Status}
		if mapping.Status == officialmodel.MappingStatusMapped {
			kind := ModelKind(mapping.ModelKind)
			candidate.ModelKind = &kind
			candidate.OfficialModelID = mapping.OfficialModelID
			candidate.OfficialCatalogVersion = mapping.CatalogVersion
			if mapping.EmbeddingSpec != nil {
				dimensions, maxInputTokens, tokenCounterID := mapping.EmbeddingSpec.Dimensions, mapping.EmbeddingSpec.MaxInputTokens, mapping.EmbeddingSpec.TokenCounterID
				candidate.EmbeddingDimensions = &dimensions
				candidate.EmbeddingMaxInputTokens = &maxInputTokens
				candidate.EmbeddingTokenCounterID = &tokenCounterID
			}
		}
		list = append(list, candidate)
	}
	return list
}

func providerModelsFromCandidates(candidates []ModelOptionDTO, mappedAt time.Time) []ProviderModel {
	models := make([]ProviderModel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ModelKind == nil || candidate.MappingStatus != officialmodel.MappingStatusMapped {
			continue
		}
		officialID, catalogVersion := candidate.OfficialModelID, candidate.OfficialCatalogVersion
		at := mappedAt.UTC()
		models = append(models, ProviderModel{
			ModelID: candidate.ModelID, ModelKind: *candidate.ModelKind, DisplayName: candidate.DisplayName,
			OfficialModelID: &officialID, OfficialCatalogVersion: &catalogVersion,
			MappingStatus: candidate.MappingStatus, MappedAt: &at,
			EmbeddingDimensions:     cloneUint32(candidate.EmbeddingDimensions),
			EmbeddingMaxInputTokens: cloneInt64(candidate.EmbeddingMaxInputTokens),
			EmbeddingTokenCounterID: cloneString(candidate.EmbeddingTokenCounterID),
			Status:                  enum.CommonYes,
		})
	}
	return models
}

func engineTypeOptions() []dict.Option[string] {
	return []dict.Option[string]{{Label: engineTypeLabels[driverOpenAI], Value: driverOpenAI}}
}

func apiProtocolOptions() []APIProtocolOption {
	options := make([]APIProtocolOption, 0, len(APIProtocols))
	for _, mode := range APIProtocols {
		options = append(options, APIProtocolOption{Label: apiProtocolLabels[mode], Value: mode})
	}
	return options
}

func isAPIProtocol(value string) bool {
	switch value {
	case APIProtocolChatCompletions, APIProtocolResponses:
		return true
	default:
		return false
	}
}

func isEngineType(value string) bool { _, ok := engineTypeLabels[value]; return ok }

func isHealthStatus(value string) bool {
	switch value {
	case provider.HealthUnknown, provider.HealthOK, provider.HealthFailed:
		return true
	default:
		return false
	}
}

func requireEngineType(value string) (string, *apperror.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", apperror.BadRequest("AI驱动不能为空")
	}
	if !isEngineType(value) {
		return "", apperror.BadRequest("无效的AI驱动")
	}
	return value, nil
}

func effectiveBaseURL(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultOpenAIBaseURL
	}
	return normalizeProviderBaseURL(driverOpenAI, value)
}

func normalizeProviderBaseURL(engineType string, value string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(value), "/")
	if engineType != driverOpenAI || baseURL == "" {
		return baseURL
	}
	normalized, err := infraai.NormalizeOpenAIBaseURL(baseURL, "")
	if err != nil {
		return baseURL
	}
	return normalized
}

func errorMessage(err error, result *infraai.TestConnectionResult) string {
	if err != nil {
		return err.Error()
	}
	if result != nil {
		return result.Message
	}
	return ""
}

func truncateErrorString(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maxStoredErrorRunes {
		return string(runes[:maxStoredErrorRunes])
	}
	return value
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

func formatPtrTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}
