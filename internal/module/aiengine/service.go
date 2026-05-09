package aiengine

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
	"admin_back_go/internal/platform/ai/provider"
	"admin_back_go/internal/platform/secretbox"
)

const (
	timeLayout                = "2006-01-02 15:04:05"
	driverOpenAI              = "openai"
	defaultOpenAIBaseURL      = "https://api.openai.com/v1"
	providerModelSourceRemote = "remote"
	maxStoredErrorRunes       = 1024
)

var engineTypeLabels = map[string]string{
	driverOpenAI: "OpenAI",
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
	tester     ConnectionTester
	driver     ModelDriver
}

func NewService(repository Repository, box secretbox.Box, tester ConnectionTester) *Service {
	return &Service{repository: repository, secretbox: box, tester: tester, driver: provider.NewOpenAIDriver(nil)}
}

func NewServiceWithDriver(repository Repository, box secretbox.Box, tester ConnectionTester, driver ModelDriver) *Service {
	service := NewService(repository, box, tester)
	if driver != nil {
		service.driver = driver
	}
	return service
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{EngineTypeArr: engineTypeOptions(), CommonStatusArr: dict.CommonStatusOptions(), HealthStatusArr: healthStatusOptions, ModelSyncArr: modelSyncStatusOptions}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	list := make([]ConnectionDTO, 0, len(rows))
	for _, row := range rows {
		models, err := repo.ListModels(ctx, row.ID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
		}
		list = append(list, connectionDTO(row, models))
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
	row, models, defaultModelID, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByTypeName(ctx, row.EngineType, row.Name, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI供应商失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("该驱动下已存在同名供应商")
	}
	ciphertext, err := s.secretbox.Encrypt(apiKey)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密AI供应商API Key失败", err)
	}
	row.APIKeyEnc = ciphertext
	row.APIKeyHint = secretbox.Hint(apiKey)
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI供应商失败", err)
	}
	if err := repo.ReplaceModels(ctx, id, models, defaultModelID); err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "保存AI供应商模型失败", err)
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	fields, models, defaultModelID, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByTypeName(ctx, strings.TrimSpace(driverFromInput(input.EngineType, input.Driver)), strings.TrimSpace(input.Name), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI供应商失败", err)
	}
	if exists {
		return apperror.BadRequest("该驱动下已存在同名供应商")
	}
	if strings.TrimSpace(input.APIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密AI供应商API Key失败", err)
		}
		fields["api_key_enc"] = ciphertext
		fields["api_key_hint"] = secretbox.Hint(strings.TrimSpace(input.APIKey))
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI供应商失败", err)
	}
	if err := repo.ReplaceModels(ctx, id, models, defaultModelID); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "保存AI供应商模型失败", err)
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI供应商状态失败", err)
	}
	return nil
}

func (s *Service) TestConnection(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI供应商ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI供应商不存在")
	}
	if row.Status != enum.CommonYes {
		return nil, apperror.BadRequest("AI供应商已禁用")
	}
	apiKey, err := s.secretbox.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	result, testErr := s.testOpenAI(ctx, row.BaseURL, apiKey)
	now := time.Now()
	health := provider.HealthOK
	message := ""
	if testErr != nil || result == nil || !result.OK {
		health = provider.HealthFailed
		message = truncateErrorString(errorMessage(testErr, result))
	}
	fields := map[string]any{"health_status": health, "last_checked_at": now, "last_check_error": message}
	if err := repo.Update(ctx, id, fields); err != nil {
		return result, apperror.Wrap(apperror.CodeInternal, 500, "更新AI供应商健康状态失败", err)
	}
	if testErr != nil {
		return result, apperror.Wrap(apperror.CodeInternal, 500, "测试AI供应商连接失败", testErr)
	}
	return result, nil
}

func (s *Service) PreviewModels(ctx context.Context, input ModelOptionsInput) (*ModelOptionsResponse, *apperror.Error) {
	driverName := normalizeDriver(driverFromInput(input.EngineType, input.Driver))
	if !isEngineType(driverName) {
		return nil, apperror.BadRequest("无效的AI驱动")
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return nil, apperror.BadRequest("API Key不能为空")
	}
	models, err := s.openAIDriver().ListModels(ctx, provider.Config{Driver: driverName, BaseURL: input.BaseURL, APIKey: apiKey, TimeoutMs: 10000})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "拉取OpenAI模型失败", err)
	}
	return &ModelOptionsResponse{List: modelOptionsDTO(models)}, nil
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI供应商不存在")
	}
	apiKey, err := s.secretbox.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	models, listErr := s.openAIDriver().ListModels(ctx, provider.Config{Driver: row.EngineType, BaseURL: row.BaseURL, APIKey: apiKey, TimeoutMs: 10000})
	now := time.Now()
	fields := map[string]any{"last_model_sync_at": now}
	if listErr != nil {
		fields["last_model_sync_status"] = provider.HealthFailed
		fields["last_model_sync_error"] = truncateErrorString(listErr.Error())
		_ = repo.Update(ctx, id, fields)
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "同步OpenAI模型失败", listErr)
	}
	fields["last_model_sync_status"] = provider.HealthOK
	fields["last_model_sync_error"] = ""
	if err := repo.Update(ctx, id, fields); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "更新AI供应商模型同步状态失败", err)
	}
	return &ModelOptionsResponse{List: modelOptionsDTO(models)}, nil
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商模型失败", err)
	}
	return &ProviderModelsResponse{List: providerModelDTOs(models)}, nil
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	models, defaultModelID, appErr := buildProviderModels(input.ModelIDs, input.DefaultModelID, input.ModelDisplayNames, input.Statuses, nil)
	if appErr != nil {
		return appErr
	}
	if err := repo.ReplaceModels(ctx, id, models, defaultModelID); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "保存AI供应商模型失败", err)
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI供应商不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI供应商失败", err)
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

func (s *Service) testOpenAI(ctx context.Context, baseURL string, apiKey string) (*platformai.TestConnectionResult, error) {
	result, err := s.openAIDriver().TestConnection(ctx, provider.Config{Driver: driverOpenAI, BaseURL: baseURL, APIKey: apiKey, TimeoutMs: 10000})
	if result == nil {
		return nil, err
	}
	return &platformai.TestConnectionResult{OK: result.OK, Status: result.Status, LatencyMs: int(result.LatencyMs), Message: result.Message}, err
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
	query.EngineType = normalizeDriver(query.EngineType)
	return query
}

func normalizeCreateInput(input CreateInput) (Connection, []ProviderModel, string, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, driverFromInput(input.EngineType, input.Driver), input.BaseURL, input.WorkspaceID, input.Status)
	if appErr != nil {
		return Connection{}, nil, "", appErr
	}
	models, defaultModelID, appErr := buildProviderModels(input.ModelIDs, input.DefaultModelID, input.ModelDisplayNames, nil, nil)
	if appErr != nil {
		return Connection{}, nil, "", appErr
	}
	return Connection{Name: fields.name, EngineType: fields.engineType, BaseURL: fields.baseURL, WorkspaceID: fields.workspaceID, Status: fields.status, HealthStatus: provider.HealthUnknown, LastModelSyncStatus: provider.HealthUnknown, IsDel: enum.CommonNo}, models, defaultModelID, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, []ProviderModel, string, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, driverFromInput(input.EngineType, input.Driver), input.BaseURL, input.WorkspaceID, input.Status)
	if appErr != nil {
		return nil, nil, "", appErr
	}
	models, defaultModelID, appErr := buildProviderModels(input.ModelIDs, input.DefaultModelID, input.ModelDisplayNames, nil, nil)
	if appErr != nil {
		return nil, nil, "", appErr
	}
	return map[string]any{"name": fields.name, "engine_type": fields.engineType, "base_url": fields.baseURL, "workspace_id": fields.workspaceID, "status": fields.status}, models, defaultModelID, nil
}

type normalizedFields struct {
	name, engineType, baseURL, workspaceID string
	status                                 int
}

func normalizeMutationFields(name, engineType, baseURL, workspaceID string, status int) (normalizedFields, *apperror.Error) {
	name = strings.TrimSpace(name)
	engineType = normalizeDriver(engineType)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	workspaceID = strings.TrimSpace(workspaceID)
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("供应商名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("供应商名称不能超过128个字符")
	}
	if !isEngineType(engineType) {
		return normalizedFields{}, apperror.BadRequest("无效的AI驱动")
	}
	if len([]rune(baseURL)) > 512 {
		return normalizedFields{}, apperror.BadRequest("供应商地址不能超过512个字符")
	}
	if len([]rune(workspaceID)) > 128 {
		return normalizedFields{}, apperror.BadRequest("工作区ID不能超过128个字符")
	}
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	return normalizedFields{name: name, engineType: engineType, baseURL: baseURL, workspaceID: workspaceID, status: status}, nil
}

func validateSelectedModels(modelIDs []string, defaultModelID string) ([]string, *apperror.Error) {
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
	if !seen[strings.TrimSpace(defaultModelID)] {
		return nil, apperror.BadRequest("默认模型必须在已选择模型中")
	}
	return normalized, nil
}

func buildProviderModels(modelIDs []string, defaultModelID string, displayNames map[string]string, statuses map[string]int, rawByID map[string]string) ([]ProviderModel, string, *apperror.Error) {
	defaultModelID = strings.TrimSpace(defaultModelID)
	normalizedIDs, appErr := validateSelectedModels(modelIDs, defaultModelID)
	if appErr != nil {
		return nil, "", appErr
	}
	models := make([]ProviderModel, 0, len(normalizedIDs))
	for _, modelID := range normalizedIDs {
		status := enum.CommonYes
		if statuses != nil && statuses[modelID] != 0 {
			status = statuses[modelID]
		}
		if !enum.IsCommonStatus(status) {
			return nil, "", apperror.BadRequest("无效的模型状态")
		}
		isDefault := enum.CommonNo
		if modelID == defaultModelID {
			isDefault = enum.CommonYes
		}
		models = append(models, ProviderModel{ModelID: modelID, DisplayName: strings.TrimSpace(displayNames[modelID]), IsDefault: isDefault, Source: providerModelSourceRemote, RawJSON: strings.TrimSpace(rawByID[modelID]), Status: status, IsDel: enum.CommonNo})
	}
	return models, defaultModelID, nil
}

func connectionDTO(row Connection, models []ProviderModel) ConnectionDTO {
	enabledCount := 0
	defaultModelID := ""
	for _, model := range models {
		if model.Status == enum.CommonYes {
			enabledCount++
		}
		if model.IsDefault == enum.CommonYes {
			defaultModelID = model.ModelID
		}
	}
	return ConnectionDTO{
		ID:                  row.ID,
		Name:                row.Name,
		EngineType:          row.EngineType,
		EngineTypeName:      engineTypeLabels[row.EngineType],
		Driver:              row.EngineType,
		DriverName:          engineTypeLabels[row.EngineType],
		BaseURL:             row.BaseURL,
		BaseURLEffective:    effectiveBaseURL(row.BaseURL),
		APIKeyMasked:        row.APIKeyHint,
		WorkspaceID:         row.WorkspaceID,
		HealthStatus:        emptyAs(row.HealthStatus, provider.HealthUnknown),
		LastCheckedAt:       formatPtrTime(row.LastCheckedAt),
		LastCheckError:      row.LastCheckError,
		LastModelSyncAt:     formatPtrTime(row.LastModelSyncAt),
		LastModelSyncStatus: emptyAs(row.LastModelSyncStatus, provider.HealthUnknown),
		LastModelSyncError:  row.LastModelSyncError,
		EnabledModelCount:   enabledCount,
		DefaultModelID:      defaultModelID,
		Models:              providerModelDTOs(models),
		Status:              row.Status,
		StatusName:          statusText(row.Status),
		CreatedAt:           formatTime(row.CreatedAt),
		UpdatedAt:           formatTime(row.UpdatedAt),
	}
}

func providerModelDTOs(rows []ProviderModel) []ProviderModelDTO {
	list := make([]ProviderModelDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, ProviderModelDTO{ID: row.ID, ProviderID: row.ProviderID, ModelID: row.ModelID, DisplayName: row.DisplayName, IsDefault: row.IsDefault, Source: row.Source, Raw: rawJSON(row.RawJSON), Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)})
	}
	return list
}

func modelOptionsDTO(models []provider.Model) []ModelOptionDTO {
	list := make([]ModelOptionDTO, 0, len(models))
	for _, model := range models {
		list = append(list, ModelOptionDTO{ModelID: model.ID, DisplayName: model.ID, OwnedBy: model.OwnedBy, Raw: rawMap(model.Raw)})
	}
	return list
}

func rawMap(value map[string]any) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return body
}

func rawJSON(value string) json.RawMessage {
	if strings.TrimSpace(value) == "" {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(value)
}

func engineTypeOptions() []dict.Option[string] {
	return []dict.Option[string]{{Label: engineTypeLabels[driverOpenAI], Value: driverOpenAI}}
}

func isEngineType(value string) bool { _, ok := engineTypeLabels[value]; return ok }

func normalizeDriver(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return driverOpenAI
	}
	return value
}

func driverFromInput(engineType string, driver string) string {
	if strings.TrimSpace(driver) != "" {
		return driver
	}
	return engineType
}

func effectiveBaseURL(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultOpenAIBaseURL
	}
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func emptyAs(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func errorMessage(err error, result *platformai.TestConnectionResult) string {
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

type unsupportedTester struct{}

func (unsupportedTester) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return nil, fmt.Errorf("ai engine tester not configured")
}
