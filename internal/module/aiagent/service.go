package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"

var agentTypeLabels = map[string]string{
	"chat":       "对话应用",
	"workflow":   "工作流",
	"completion": "文本生成",
	"agent":      "智能体",
}

var sceneLabels = map[string]string{
	"chat": "对话",
}

var responseModeLabels = map[string]string{
	"streaming": "流式",
	"blocking":  "阻塞",
}

var bindingTypeLabels = map[string]string{
	"menu":       "菜单",
	"scene":      "场景",
	"permission": "权限",
	"role":       "角色",
	"user":       "用户",
}

type Service struct {
	repository Repository
	secretbox  secretbox.Box
	tester     ConnectionTester
}

func NewService(repository Repository, box secretbox.Box, tester ConnectionTester) *Service {
	return &Service{repository: repository, secretbox: box, tester: tester}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	connections, err := repo.ListActiveProviders(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商选项失败", err)
	}
	options := make([]EngineOption, 0, len(connections))
	modelOptions := []ModelOption{}
	for _, row := range connections {
		options = append(options, EngineOption{Label: row.Name, Value: row.ID, EngineType: row.EngineType})
		models, err := repo.ListProviderModels(ctx, row.ID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
		}
		for _, model := range models {
			if model.Status != enum.CommonYes {
				continue
			}
			label := strings.TrimSpace(model.DisplayName)
			if label == "" {
				label = model.ModelID
			}
			modelOptions = append(modelOptions, ModelOption{Label: label, Value: model.ModelID, ProviderID: row.ID, ModelID: model.ModelID, DisplayName: model.DisplayName})
		}
	}
	return &InitResponse{Dict: InitDict{AgentTypeArr: agentTypeOptions(), SceneArr: sceneOptions(), ResponseModeArr: responseModeOptions(), BindingTypeArr: bindingTypeOptions(), CommonStatusArr: dict.CommonStatusOptions(), ProviderOptions: options, ModelOptions: modelOptions}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	list := make([]AgentDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, agentDTO(row))
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	list := make([]ProviderModelDTO, 0, len(rows))
	for _, row := range rows {
		if row.Status != enum.CommonYes {
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
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI智能体不存在")
	}
	return &DetailResponse{AgentDTO: agentDTO(*row)}, nil
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
	model, appErr := s.ensureProviderModel(ctx, repo, row.ProviderID, row.ModelID)
	if appErr != nil {
		return 0, appErr
	}
	row.ModelDisplayName = model.DisplayName
	row.ModelSnapshotJSON = modelSnapshotJSON(*model)
	exists, err := repo.ExistsByCode(ctx, row.Code, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI智能体编码失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("AI智能体编码已存在")
	}
	if strings.TrimSpace(input.ExternalAgentAPIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.ExternalAgentAPIKey))
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密AI智能体API Key失败", err)
		}
		row.ExternalAgentAPIKeyEnc = ciphertext
		row.ExternalAgentAPIKeyHint = secretbox.Hint(strings.TrimSpace(input.ExternalAgentAPIKey))
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI智能体失败", err)
	}
	return id, nil
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return appErr
	}
	if appErr := s.ensureActiveProvider(ctx, repo, input.ProviderID); appErr != nil {
		return appErr
	}
	model, appErr := s.ensureProviderModel(ctx, repo, input.ProviderID, fields.modelID)
	if appErr != nil {
		return appErr
	}
	fields.modelDisplayName = model.DisplayName
	fields.modelSnapshotJSON = modelSnapshotJSON(*model)
	updateFields := updateFieldsMap(fields)
	exists, err := repo.ExistsByCode(ctx, strings.TrimSpace(input.Code), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI智能体编码失败", err)
	}
	if exists {
		return apperror.BadRequest("AI智能体编码已存在")
	}
	if strings.TrimSpace(input.ExternalAgentAPIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.ExternalAgentAPIKey))
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密AI智能体API Key失败", err)
		}
		updateFields["external_agent_api_key_enc"] = ciphertext
		updateFields["external_agent_api_key_hint"] = secretbox.Hint(strings.TrimSpace(input.ExternalAgentAPIKey))
	}
	if err := repo.Update(ctx, id, updateFields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI智能体失败", err)
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI智能体状态失败", err)
	}
	return nil
}

func (s *Service) Test(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI智能体不存在")
	}
	if row.Status != enum.CommonYes {
		return nil, apperror.BadRequest("AI智能体已禁用")
	}
	connection, err := repo.GetActiveProvider(ctx, row.ProviderID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return nil, apperror.BadRequest("AI供应商不存在或已禁用")
	}
	apiKey, err := s.secretbox.Decrypt(row.ExternalAgentAPIKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI智能体API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI智能体API Key未配置")
	}
	tester := s.tester
	if tester == nil {
		tester = unsupportedTester{}
	}
	result, testErr := tester.TestConnection(ctx, platformai.TestConnectionInput{EngineType: platformai.EngineType(connection.EngineType), BaseURL: connection.BaseURL, APIKey: apiKey, TimeoutMs: 10000})
	if testErr != nil {
		return result, apperror.Wrap(apperror.CodeInternal, 500, "测试AI智能体失败", testErr)
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI智能体失败", err)
	}
	return nil
}

func (s *Service) Bindings(ctx context.Context, agentID uint64) (*BindingListResponse, *apperror.Error) {
	if agentID == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := ensureAgentExists(ctx, repo, agentID); appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListBindings(ctx, agentID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体绑定失败", err)
	}
	list := make([]BindingDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, bindingDTO(row))
	}
	return &BindingListResponse{List: list}, nil
}

func (s *Service) CreateBinding(ctx context.Context, agentID uint64, input BindingInput) (uint64, *apperror.Error) {
	if agentID == 0 {
		return 0, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	if appErr := ensureAgentExists(ctx, repo, agentID); appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeBindingInput(agentID, input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsBinding(ctx, agentID, row.BindType, row.BindKey, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI智能体绑定失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("AI智能体绑定已存在")
	}
	id, err := repo.CreateBinding(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI智能体绑定失败", err)
	}
	return id, nil
}

func (s *Service) DeleteBinding(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI智能体绑定ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.DeleteBinding(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI智能体绑定失败", err)
	}
	return nil
}

func (s *Service) Options(ctx context.Context, query OptionQuery) (*AgentOptionsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListVisibleAgents(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询可用AI智能体失败", err)
	}
	list := make([]AgentOption, 0, len(rows))
	for _, row := range rows {
		if row.Status != enum.CommonYes || row.IsDel == enum.CommonYes {
			continue
		}
		list = append(list, AgentOption{Label: row.Name, Value: row.ID, Code: row.Code})
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return apperror.BadRequest("AI供应商不存在或已禁用")
	}
	return nil
}

func (s *Service) ensureProviderModel(ctx context.Context, repo Repository, providerID uint64, modelID string) (*ProviderModel, *apperror.Error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, apperror.BadRequest("关联模型不能为空")
	}
	models, err := repo.ListProviderModels(ctx, providerID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	for _, model := range models {
		if model.Status == enum.CommonYes && strings.TrimSpace(model.ModelID) == modelID {
			return &model, nil
		}
	}
	return nil, apperror.BadRequest("关联模型不存在或已禁用")
}

func ensureAgentExists(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
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
	query.Name = strings.TrimSpace(query.Name)
	query.Code = strings.TrimSpace(query.Code)
	query.AgentType = strings.TrimSpace(query.AgentType)
	return query
}

func normalizeCreateInput(input CreateInput) (Agent, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return Agent{}, appErr
	}
	return Agent{
		ProviderID:          fields.providerID,
		Name:                fields.name,
		Code:                fields.code,
		AgentType:           fields.agentType,
		ModelID:             fields.modelID,
		ScenesJSON:          fields.scenesJSON,
		SystemPrompt:        fields.systemPrompt,
		Avatar:              fields.avatar,
		ExternalAgentID:     fields.externalAgentID,
		DefaultResponseMode: fields.defaultResponseMode,
		RuntimeConfigJSON:   fields.runtimeConfigJSON,
		ModelSnapshotJSON:   "{}",
		Status:              fields.status,
		IsDel:               enum.CommonNo,
	}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return nil, appErr
	}
	return updateFieldsMap(fields), nil
}

func updateFieldsMap(fields normalizedFields) map[string]any {
	out := map[string]any{
		"provider_id":           fields.providerID,
		"name":                  fields.name,
		"code":                  fields.code,
		"agent_type":            fields.agentType,
		"model_id":              fields.modelID,
		"scenes_json":           fields.scenesJSON,
		"system_prompt":         fields.systemPrompt,
		"avatar":                fields.avatar,
		"external_agent_id":     fields.externalAgentID,
		"default_response_mode": fields.defaultResponseMode,
		"runtime_config_json":   fields.runtimeConfigJSON,
		"status":                fields.status,
	}
	if fields.modelDisplayName != "" {
		out["model_display_name"] = fields.modelDisplayName
	}
	if fields.modelSnapshotJSON != "" {
		out["model_snapshot_json"] = fields.modelSnapshotJSON
	}
	return out
}

type normalizedFields struct {
	providerID          uint64
	name                string
	code                string
	agentType           string
	modelID             string
	modelDisplayName    string
	scenesJSON          string
	systemPrompt        string
	avatar              string
	externalAgentID     string
	defaultResponseMode string
	runtimeConfigJSON   string
	modelSnapshotJSON   string
	status              int
}

func normalizeMutationFields(input CreateInput) (normalizedFields, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	agentType := strings.TrimSpace(input.AgentType)
	modelID := strings.TrimSpace(input.ModelID)
	systemPrompt := strings.TrimSpace(input.SystemPrompt)
	avatar := strings.TrimSpace(input.Avatar)
	externalAgentID := strings.TrimSpace(input.ExternalAgentID)
	defaultResponseMode := strings.TrimSpace(input.DefaultResponseMode)
	if input.ProviderID == 0 {
		return normalizedFields{}, apperror.BadRequest("AI供应商不能为空")
	}
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("AI智能体名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI智能体名称不能超过128个字符")
	}
	if code == "" {
		return normalizedFields{}, apperror.BadRequest("AI智能体编码不能为空")
	}
	if len([]rune(code)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI智能体编码不能超过128个字符")
	}
	if modelID == "" {
		return normalizedFields{}, apperror.BadRequest("关联模型不能为空")
	}
	if len([]rune(modelID)) > 191 {
		return normalizedFields{}, apperror.BadRequest("关联模型不能超过191个字符")
	}
	if agentType == "" {
		agentType = "chat"
	}
	if !isAgentType(agentType) {
		return normalizedFields{}, apperror.BadRequest("无效的AI智能体类型")
	}
	scenesJSON, appErr := encodeScenes(input.Scenes)
	if appErr != nil {
		return normalizedFields{}, appErr
	}
	if len([]rune(systemPrompt)) > 20000 {
		return normalizedFields{}, apperror.BadRequest("系统提示词不能超过20000个字符")
	}
	if len([]rune(avatar)) > 512 {
		return normalizedFields{}, apperror.BadRequest("头像地址不能超过512个字符")
	}
	if len([]rune(externalAgentID)) > 128 {
		return normalizedFields{}, apperror.BadRequest("引擎应用ID不能超过128个字符")
	}
	if defaultResponseMode == "" {
		defaultResponseMode = "streaming"
	}
	if !isResponseMode(defaultResponseMode) {
		return normalizedFields{}, apperror.BadRequest("无效的响应模式")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	runtimeJSON, appErr := encodeJSONMap(input.RuntimeConfig)
	if appErr != nil {
		return normalizedFields{}, appErr
	}
	return normalizedFields{providerID: input.ProviderID, name: name, code: code, agentType: agentType, modelID: modelID, scenesJSON: scenesJSON, systemPrompt: systemPrompt, avatar: avatar, externalAgentID: externalAgentID, defaultResponseMode: defaultResponseMode, runtimeConfigJSON: runtimeJSON, status: status}, nil
}

func normalizeBindingInput(agentID uint64, input BindingInput) (Binding, *apperror.Error) {
	bindType := strings.TrimSpace(input.BindType)
	bindKey := strings.TrimSpace(input.BindKey)
	if !isBindingType(bindType) {
		return Binding{}, apperror.BadRequest("无效的AI智能体绑定类型")
	}
	if bindKey == "" {
		return Binding{}, apperror.BadRequest("AI智能体绑定键不能为空")
	}
	if len([]rune(bindKey)) > 128 {
		return Binding{}, apperror.BadRequest("AI智能体绑定键不能超过128个字符")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return Binding{}, apperror.BadRequest("无效的状态")
	}
	return Binding{AgentID: agentID, BindType: bindType, BindKey: bindKey, Sort: input.Sort, Status: status, IsDel: enum.CommonNo}, nil
}

func agentDTO(row AgentWithProvider) AgentDTO {
	scenes := decodeScenes(row.ScenesJSON)
	return AgentDTO{ID: row.ID, ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType, Name: row.Name, Code: row.Code, ModelID: row.ModelID, ModelDisplayName: row.ModelDisplayName, Scenes: scenes, SceneNames: sceneNames(scenes), SystemPrompt: row.SystemPrompt, Avatar: row.Avatar, AgentType: row.AgentType, AgentTypeName: agentTypeLabels[row.AgentType], ExternalAgentID: row.ExternalAgentID, ExternalAgentAPIKeyMasked: row.ExternalAgentAPIKeyHint, DefaultResponseMode: row.DefaultResponseMode, DefaultResponseModeName: responseModeLabels[row.DefaultResponseMode], RuntimeConfig: decodeJSONMap(row.RuntimeConfigJSON), Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func bindingDTO(row Binding) BindingDTO {
	return BindingDTO{ID: row.ID, AgentID: row.AgentID, BindType: row.BindType, BindTypeName: bindingTypeLabels[row.BindType], BindKey: row.BindKey, Sort: row.Sort, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func providerModelDTO(row ProviderModel) ProviderModelDTO {
	return ProviderModelDTO{ID: row.ID, ProviderID: row.ProviderID, ModelID: row.ModelID, DisplayName: row.DisplayName, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func encodeJSONMap(value map[string]any) (string, *apperror.Error) {
	if value == nil {
		return "{}", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", apperror.BadRequest("运行配置不是合法JSON")
	}
	return string(data), nil
}

func decodeJSONMap(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" || value == "null" {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(value), &result); err != nil || result == nil {
		return map[string]any{}
	}
	return result
}

func agentTypeOptions() []dict.Option[string] {
	return stringOptions([]string{"chat", "workflow", "completion", "agent"}, agentTypeLabels)
}
func sceneOptions() []dict.Option[string] {
	return stringOptions([]string{"chat"}, sceneLabels)
}
func responseModeOptions() []dict.Option[string] {
	return stringOptions([]string{"streaming", "blocking"}, responseModeLabels)
}
func bindingTypeOptions() []dict.Option[string] {
	return stringOptions([]string{"menu", "scene", "permission", "role", "user"}, bindingTypeLabels)
}

func stringOptions(values []string, labels map[string]string) []dict.Option[string] {
	options := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, dict.Option[string]{Label: labels[value], Value: value})
	}
	return options
}

func isAgentType(value string) bool    { _, ok := agentTypeLabels[value]; return ok }
func isResponseMode(value string) bool { _, ok := responseModeLabels[value]; return ok }
func isBindingType(value string) bool  { _, ok := bindingTypeLabels[value]; return ok }
func isScene(value string) bool        { _, ok := sceneLabels[value]; return ok }

func encodeScenes(values []string) (string, *apperror.Error) {
	if len(values) == 0 {
		values = []string{"chat"}
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
		normalized = []string{"chat"}
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
		return []string{"chat"}
	}
	var scenes []string
	if err := json.Unmarshal([]byte(value), &scenes); err != nil || len(scenes) == 0 {
		return []string{"chat"}
	}
	out := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		scene = strings.TrimSpace(scene)
		if isScene(scene) {
			out = append(out, scene)
		}
	}
	if len(out) == 0 {
		return []string{"chat"}
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

func modelSnapshotJSON(model ProviderModel) string {
	displayName := strings.TrimSpace(model.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(model.ModelID)
	}
	data, err := json.Marshal(map[string]any{
		"model_id":     strings.TrimSpace(model.ModelID),
		"display_name": displayName,
	})
	if err != nil {
		return "{}"
	}
	return string(data)
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

func (unsupportedTester) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return nil, fmt.Errorf("ai agent tester not configured")
}
