package aiapp

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

var appTypeLabels = map[string]string{
	"chat":       "对话应用",
	"workflow":   "工作流",
	"completion": "文本生成",
	"agent":      "智能体",
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
	for _, row := range connections {
		options = append(options, EngineOption{Label: row.Name, Value: row.ID, EngineType: row.EngineType})
	}
	return &InitResponse{Dict: InitDict{AppTypeArr: appTypeOptions(), ResponseModeArr: responseModeOptions(), BindingTypeArr: bindingTypeOptions(), CommonStatusArr: dict.CommonStatusOptions(), ProviderOptions: options}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	list := make([]AppDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, appDTO(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI应用ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI应用不存在")
	}
	return &DetailResponse{AppDTO: appDTO(*row)}, nil
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
	exists, err := repo.ExistsByCode(ctx, row.Code, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI应用编码失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("AI应用编码已存在")
	}
	if strings.TrimSpace(input.EngineAppAPIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.EngineAppAPIKey))
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密AI应用API Key失败", err)
		}
		row.EngineAppAPIKeyEnc = ciphertext
		row.EngineAppAPIKeyHint = secretbox.Hint(strings.TrimSpace(input.EngineAppAPIKey))
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI应用失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI应用ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI应用不存在")
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	if appErr := s.ensureActiveProvider(ctx, repo, input.ProviderID); appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByCode(ctx, strings.TrimSpace(input.Code), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI应用编码失败", err)
	}
	if exists {
		return apperror.BadRequest("AI应用编码已存在")
	}
	if strings.TrimSpace(input.EngineAppAPIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.EngineAppAPIKey))
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密AI应用API Key失败", err)
		}
		fields["engine_app_api_key_enc"] = ciphertext
		fields["engine_app_api_key_hint"] = secretbox.Hint(strings.TrimSpace(input.EngineAppAPIKey))
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI应用失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI应用ID")
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI应用不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI应用状态失败", err)
	}
	return nil
}

func (s *Service) Test(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI应用ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI应用不存在")
	}
	if row.Status != enum.CommonYes {
		return nil, apperror.BadRequest("AI应用已禁用")
	}
	connection, err := repo.GetActiveProvider(ctx, row.ProviderID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return nil, apperror.BadRequest("AI供应商不存在或已禁用")
	}
	apiKey, err := s.secretbox.Decrypt(row.EngineAppAPIKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI应用API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI应用API Key未配置")
	}
	tester := s.tester
	if tester == nil {
		tester = unsupportedTester{}
	}
	result, testErr := tester.TestConnection(ctx, platformai.TestConnectionInput{EngineType: platformai.EngineType(connection.EngineType), BaseURL: connection.BaseURL, APIKey: apiKey, TimeoutMs: 10000})
	if testErr != nil {
		return result, apperror.Wrap(apperror.CodeInternal, 500, "测试AI应用失败", testErr)
	}
	return result, nil
}

func (s *Service) Delete(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI应用ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI应用不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI应用失败", err)
	}
	return nil
}

func (s *Service) Bindings(ctx context.Context, appID uint64) (*BindingListResponse, *apperror.Error) {
	if appID == 0 {
		return nil, apperror.BadRequest("无效的AI应用ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := ensureAppExists(ctx, repo, appID); appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListBindings(ctx, appID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用绑定失败", err)
	}
	list := make([]BindingDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, bindingDTO(row))
	}
	return &BindingListResponse{List: list}, nil
}

func (s *Service) CreateBinding(ctx context.Context, appID uint64, input BindingInput) (uint64, *apperror.Error) {
	if appID == 0 {
		return 0, apperror.BadRequest("无效的AI应用ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	if appErr := ensureAppExists(ctx, repo, appID); appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeBindingInput(appID, input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsBinding(ctx, appID, row.BindType, row.BindKey, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI应用绑定失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("AI应用绑定已存在")
	}
	id, err := repo.CreateBinding(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI应用绑定失败", err)
	}
	return id, nil
}

func (s *Service) DeleteBinding(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI应用绑定ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.DeleteBinding(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI应用绑定失败", err)
	}
	return nil
}

func (s *Service) Options(ctx context.Context, query OptionQuery) (*AppOptionsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListVisibleApps(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询可用AI应用失败", err)
	}
	list := make([]AppOption, 0, len(rows))
	for _, row := range rows {
		if row.Status != enum.CommonYes || row.IsDel == enum.CommonYes {
			continue
		}
		list = append(list, AppOption{Label: row.Name, Value: row.ID, Code: row.Code})
	}
	return &AppOptionsResponse{List: list}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI应用仓储未配置")
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

func ensureAppExists(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI应用不存在")
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
	query.AppType = strings.TrimSpace(query.AppType)
	return query
}

func normalizeCreateInput(input CreateInput) (App, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return App{}, appErr
	}
	return App{ProviderID: fields.providerID, Name: fields.name, Code: fields.code, AppType: fields.appType, EngineAppID: fields.engineAppID, DefaultResponseMode: fields.defaultResponseMode, RuntimeConfigJSON: fields.runtimeConfigJSON, ModelSnapshotJSON: "{}", Status: fields.status, IsDel: enum.CommonNo}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{"provider_id": fields.providerID, "name": fields.name, "code": fields.code, "app_type": fields.appType, "engine_app_id": fields.engineAppID, "default_response_mode": fields.defaultResponseMode, "runtime_config_json": fields.runtimeConfigJSON, "status": fields.status}, nil
}

type normalizedFields struct {
	providerID          uint64
	name                string
	code                string
	appType             string
	engineAppID         string
	defaultResponseMode string
	runtimeConfigJSON   string
	status              int
}

func normalizeMutationFields(input CreateInput) (normalizedFields, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	appType := strings.TrimSpace(input.AppType)
	engineAppID := strings.TrimSpace(input.EngineAppID)
	defaultResponseMode := strings.TrimSpace(input.DefaultResponseMode)
	if input.ProviderID == 0 {
		return normalizedFields{}, apperror.BadRequest("AI供应商不能为空")
	}
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("AI应用名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI应用名称不能超过128个字符")
	}
	if code == "" {
		return normalizedFields{}, apperror.BadRequest("AI应用编码不能为空")
	}
	if len([]rune(code)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI应用编码不能超过128个字符")
	}
	if !isAppType(appType) {
		return normalizedFields{}, apperror.BadRequest("无效的AI应用类型")
	}
	if len([]rune(engineAppID)) > 128 {
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
	return normalizedFields{providerID: input.ProviderID, name: name, code: code, appType: appType, engineAppID: engineAppID, defaultResponseMode: defaultResponseMode, runtimeConfigJSON: runtimeJSON, status: status}, nil
}

func normalizeBindingInput(appID uint64, input BindingInput) (Binding, *apperror.Error) {
	bindType := strings.TrimSpace(input.BindType)
	bindKey := strings.TrimSpace(input.BindKey)
	if !isBindingType(bindType) {
		return Binding{}, apperror.BadRequest("无效的AI应用绑定类型")
	}
	if bindKey == "" {
		return Binding{}, apperror.BadRequest("AI应用绑定键不能为空")
	}
	if len([]rune(bindKey)) > 128 {
		return Binding{}, apperror.BadRequest("AI应用绑定键不能超过128个字符")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return Binding{}, apperror.BadRequest("无效的状态")
	}
	return Binding{AppID: appID, BindType: bindType, BindKey: bindKey, Sort: input.Sort, Status: status, IsDel: enum.CommonNo}, nil
}

func appDTO(row AppWithEngine) AppDTO {
	return AppDTO{ID: row.ID, ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType, Name: row.Name, Code: row.Code, AppType: row.AppType, AppTypeName: appTypeLabels[row.AppType], EngineAppID: row.EngineAppID, EngineAppAPIKeyMasked: row.EngineAppAPIKeyHint, DefaultResponseMode: row.DefaultResponseMode, DefaultResponseModeName: responseModeLabels[row.DefaultResponseMode], RuntimeConfig: decodeJSONMap(row.RuntimeConfigJSON), Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func bindingDTO(row Binding) BindingDTO {
	return BindingDTO{ID: row.ID, AppID: row.AppID, BindType: row.BindType, BindTypeName: bindingTypeLabels[row.BindType], BindKey: row.BindKey, Sort: row.Sort, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
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

func appTypeOptions() []dict.Option[string] {
	return stringOptions([]string{"chat", "workflow", "completion", "agent"}, appTypeLabels)
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

func isAppType(value string) bool      { _, ok := appTypeLabels[value]; return ok }
func isResponseMode(value string) bool { _, ok := responseModeLabels[value]; return ok }
func isBindingType(value string) bool  { _, ok := bindingTypeLabels[value]; return ok }

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
	return nil, fmt.Errorf("ai app tester not configured")
}
