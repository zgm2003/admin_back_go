package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	sharedmoney "admin_back_go/internal/shared/money"
)

const (
	timeLayout         = "2006-01-02 15:04:05"
	sceneChat          = "chat"
	sceneAgentGenerate = "agent_generate"
)

var sceneLabels = map[string]string{
	sceneChat:                     "对话",
	sceneAgentGenerate:            "工具生成",
	capability.SceneTextGenerate:  "文本生成",
	capability.SceneImageGenerate: "图片生成",
}

type Service struct {
	repository Repository
	secretbox  secretbox.Box
	tester     ConnectionTester
}

func NewService(repository Repository, box secretbox.Box, tester ConnectionTester) *Service {
	return &Service{repository: repository, secretbox: box, tester: tester}
}

func (s *Service) PageInit(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
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
			if model.Status != enum.CommonYes {
				continue
			}
			label := strings.TrimSpace(model.DisplayName)
			if label == "" {
				label = model.ModelID
			}
			option := ModelOption{Label: label, Value: model.ModelID, ProviderID: row.ID, ModelID: model.ModelID, DisplayName: model.DisplayName, BillingMultiplier: "1", MaxOutputTokens: 4096}
			if catalogModel, resolveErr := pricing.Default.Resolve(model.ModelID); resolveErr == nil {
				option.CatalogVersion, option.CatalogVendor, option.CatalogModelID = catalogModel.Version, catalogModel.CatalogVendor, catalogModel.ModelID
				option.CatalogRates = catalogRates(catalogModel)
			}
			modelOptions = append(modelOptions, option)
		}
	}
	return &InitResponse{Dict: InitDict{SceneArr: sceneOptions(), CommonStatusArr: dict.CommonStatusOptions(), ProviderOptions: options, ModelOptions: modelOptions, BillingMultiplierDefault: "1", MaxOutputTokensDefault: 4096}}, nil
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
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
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
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
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
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
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
	if appErr := validateCatalogOutput(row.ModelID, row.MaxOutputTokens); appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "新增AI智能体失败", err)
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
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if strings.TrimSpace(input.BillingMultiplier) == "" {
		input.BillingMultiplier = formatMultiplier(defaultMultiplier(row.BillingMultiplierPPM))
	}
	if input.MaxOutputTokens == 0 {
		input.MaxOutputTokens = int(defaultMaxOutput(row.MaxOutputTokens))
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
	if appErr := validateCatalogOutput(fields.modelID, fields.maxOutputTokens); appErr != nil {
		return appErr
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
	rows, err := repo.ListVisibleAgents(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询可用AI智能体失败", err)
	}
	list := make([]AgentOption, 0, len(rows))
	for _, row := range rows {
		if row.Status != enum.CommonYes || row.IsDel == enum.CommonYes {
			continue
		}
		list = append(list, AgentOption{ID: row.ID, Name: row.Name, Avatar: row.Avatar, SystemPrompt: row.SystemPrompt})
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

func (s *Service) ensureProviderModel(ctx context.Context, repo Repository, providerID uint64, modelID string) (*ProviderModel, *apperror.Error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, apperror.BadRequest("关联模型不能为空")
	}
	models, err := repo.ListProviderModels(ctx, providerID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	for _, model := range models {
		if model.Status == enum.CommonYes && strings.TrimSpace(model.ModelID) == modelID {
			return &model, nil
		}
	}
	return nil, apperror.BadRequest("关联模型不存在或已禁用")
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
		MaxOutputTokens:      fields.maxOutputTokens,
	}, nil
}

func updateFieldsMap(fields normalizedFields) map[string]any {
	out := map[string]any{
		"provider_id":            fields.providerID,
		"name":                   fields.name,
		"model_id":               fields.modelID,
		"scenes_json":            fields.scenesJSON,
		"system_prompt":          fields.systemPrompt,
		"avatar":                 fields.avatar,
		"status":                 fields.status,
		"billing_multiplier_ppm": fields.billingMultiplierPPM,
		"max_output_tokens":      fields.maxOutputTokens,
	}
	if fields.modelDisplayName != "" {
		out["model_display_name"] = fields.modelDisplayName
	}
	return out
}

type normalizedFields struct {
	providerID           uint64
	name                 string
	modelID              string
	modelDisplayName     string
	scenesJSON           string
	systemPrompt         string
	avatar               string
	status               int
	billingMultiplierPPM int64
	maxOutputTokens      int64
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
	maxOutput := int64(input.MaxOutputTokens)
	if input.MaxOutputTokens == 0 {
		maxOutput = 4096
	}
	if input.MaxOutputTokens < 0 || maxOutput <= 0 {
		return normalizedFields{}, apperror.BadRequest("max_output_tokens必须为正数")
	}
	if maxOutput > pricing.MaxSafeOutputTokens {
		return normalizedFields{}, apperror.BadRequest("max_output_tokens超过安全上限")
	}
	return normalizedFields{providerID: input.ProviderID, name: name, modelID: modelID, scenesJSON: scenesJSON, systemPrompt: systemPrompt, avatar: avatar, status: status, billingMultiplierPPM: multiplier, maxOutputTokens: maxOutput}, nil
}

func agentDTO(row AgentWithProvider) AgentDTO {
	scenes := decodeScenes(row.ScenesJSON)
	multiplier := defaultMultiplier(row.BillingMultiplierPPM)
	maxOutput := defaultMaxOutput(row.MaxOutputTokens)
	dto := AgentDTO{ID: row.ID, ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType, Name: row.Name, ModelID: row.ModelID, ModelDisplayName: row.ModelDisplayName, Scenes: scenes, SceneNames: sceneNames(scenes), SystemPrompt: row.SystemPrompt, Avatar: row.Avatar, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt), BillingMultiplier: formatMultiplier(multiplier), MaxOutputTokens: int(maxOutput)}
	if model, err := pricing.Default.Resolve(row.ModelID); err == nil {
		dto.CatalogVersion, dto.CatalogVendor, dto.CatalogModelID = model.Version, model.CatalogVendor, model.ModelID
		dto.CatalogRates = catalogRates(model)
	}
	return dto
}

func catalogRates(model pricing.ModelPrice) []CatalogRateDTO {
	rates := make([]CatalogRateDTO, 0, len(model.Rates))
	for _, rate := range model.Rates {
		formatted, formatErr := sharedmoney.FormatRMBUnits(rate.PriceUnits)
		if formatErr == nil {
			rates = append(rates, CatalogRateDTO{Category: string(rate.Category), Unit: rate.Unit, TierKey: rate.TierKey, Price: formatted, UnitScale: rate.UnitScale})
		}
	}
	return rates
}

func validateCatalogOutput(modelID string, maxOutput int64) *apperror.Error {
	model, err := pricing.Default.Resolve(modelID)
	if err != nil {
		return nil
	}
	if model.MaxOutputTokens > 0 && maxOutput > model.MaxOutputTokens {
		return apperror.BadRequest("max_output_tokens超过官方模型上限")
	}
	return nil
}

func defaultMultiplier(value int64) int64 {
	if value <= 0 {
		return 1000000
	}
	return value
}
func defaultMaxOutput(value int64) int64 {
	if value <= 0 {
		return 4096
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
	return ProviderModelDTO{ID: row.ID, ProviderID: row.ProviderID, ModelID: row.ModelID, DisplayName: row.DisplayName, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
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
