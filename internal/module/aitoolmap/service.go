package aitoolmap

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const (
	timeLayout = "2006-01-02 15:04:05"

	ToolTypeDifyTool           = "dify_tool"
	ToolTypeWorkflowNode       = "workflow_node"
	ToolTypeAdminActionGateway = "admin_action_gateway"
	ToolTypeHTTPReference      = "http_reference"

	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

var toolTypeLabels = map[string]string{
	ToolTypeDifyTool:           "Dify工具",
	ToolTypeWorkflowNode:       "工作流节点",
	ToolTypeAdminActionGateway: "后台动作网关",
	ToolTypeHTTPReference:      "HTTP引用",
}

var riskLevelLabels = map[string]string{
	RiskLow:    "低",
	RiskMedium: "中",
	RiskHigh:   "高",
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

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
	return &InitResponse{Dict: InitDict{
		ToolTypeArr:     stringOptions([]string{ToolTypeDifyTool, ToolTypeWorkflowNode, ToolTypeAdminActionGateway, ToolTypeHTTPReference}, toolTypeLabels),
		RiskLevelArr:    stringOptions([]string{RiskLow, RiskMedium, RiskHigh}, riskLevelLabels),
		CommonStatusArr: dict.CommonStatusOptions(),
		ProviderOptions: options,
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具映射失败", err)
	}
	list := make([]ToolMapDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, toolMapDTO(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Create(ctx context.Context, input MutationInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := s.normalizeMutation(ctx, repo, input, 0)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI工具映射失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id uint64, input MutationInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI工具映射ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	row, appErr := s.normalizeMutation(ctx, repo, input, id)
	if appErr != nil {
		return appErr
	}
	fields := map[string]any{
		"provider_id":     row.ProviderID,
		"app_id":          row.AppID,
		"name":            row.Name,
		"code":            row.Code,
		"tool_type":       row.ToolType,
		"engine_tool_id":  row.EngineToolID,
		"permission_code": row.PermissionCode,
		"risk_level":      row.RiskLevel,
		"config_json":     row.ConfigJSON,
		"status":          row.Status,
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI工具映射失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI工具映射ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI工具映射状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI工具映射ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI工具映射失败", err)
	}
	return nil
}

func (s *Service) normalizeMutation(ctx context.Context, repo Repository, input MutationInput, excludeID uint64) (ToolMap, *apperror.Error) {
	fields, appErr := normalizeFields(input)
	if appErr != nil {
		return ToolMap{}, appErr
	}
	connection, err := repo.GetActiveProvider(ctx, fields.providerID)
	if err != nil {
		return ToolMap{}, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return ToolMap{}, apperror.BadRequest("AI供应商不存在或已禁用")
	}
	exists, err := repo.ExistsByCode(ctx, fields.code, excludeID)
	if err != nil {
		return ToolMap{}, apperror.Wrap(apperror.CodeInternal, 500, "校验AI工具映射编码失败", err)
	}
	if exists {
		return ToolMap{}, apperror.BadRequest("AI工具映射编码已存在")
	}
	if fields.toolType == ToolTypeAdminActionGateway {
		if fields.permissionCode == "" {
			return ToolMap{}, apperror.BadRequest("admin_action_gateway 必须绑定有效权限编码")
		}
		ok, err := repo.ExistsPermissionCode(ctx, fields.permissionCode)
		if err != nil {
			return ToolMap{}, apperror.Wrap(apperror.CodeInternal, 500, "校验权限编码失败", err)
		}
		if !ok {
			return ToolMap{}, apperror.BadRequest("权限编码不存在或已禁用")
		}
	}
	return ToolMap{
		ProviderID:     fields.providerID,
		AppID:          fields.appID,
		Name:           fields.name,
		Code:           fields.code,
		ToolType:       fields.toolType,
		EngineToolID:   fields.engineToolID,
		PermissionCode: fields.permissionCode,
		RiskLevel:      fields.riskLevel,
		ConfigJSON:     fields.configJSON,
		Status:         fields.status,
		IsDel:          enum.CommonNo,
	}, nil
}

type normalizedFields struct {
	providerID     uint64
	appID          *uint64
	name           string
	code           string
	toolType       string
	engineToolID   string
	permissionCode string
	riskLevel      string
	configJSON     string
	status         int
}

func normalizeFields(input MutationInput) (normalizedFields, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	toolType := strings.TrimSpace(input.ToolType)
	engineToolID := strings.TrimSpace(input.EngineToolID)
	permissionCode := strings.TrimSpace(input.PermissionCode)
	riskLevel := strings.TrimSpace(input.RiskLevel)
	if input.ProviderID == 0 {
		return normalizedFields{}, apperror.BadRequest("AI供应商不能为空")
	}
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("AI工具映射名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI工具映射名称不能超过128个字符")
	}
	if code == "" {
		return normalizedFields{}, apperror.BadRequest("AI工具映射编码不能为空")
	}
	if len([]rune(code)) > 128 {
		return normalizedFields{}, apperror.BadRequest("AI工具映射编码不能超过128个字符")
	}
	if !isToolType(toolType) {
		return normalizedFields{}, apperror.BadRequest("无效的AI工具类型")
	}
	if len([]rune(engineToolID)) > 128 {
		return normalizedFields{}, apperror.BadRequest("引擎工具ID不能超过128个字符")
	}
	if len([]rune(permissionCode)) > 128 {
		return normalizedFields{}, apperror.BadRequest("权限编码不能超过128个字符")
	}
	if riskLevel == "" {
		riskLevel = RiskLow
	}
	if !isRiskLevel(riskLevel) {
		return normalizedFields{}, apperror.BadRequest("无效的风险等级")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	configJSON, appErr := normalizeRawJSON(input.ConfigJSON)
	if appErr != nil {
		return normalizedFields{}, appErr
	}
	return normalizedFields{providerID: input.ProviderID, appID: input.AppID, name: name, code: code, toolType: toolType, engineToolID: engineToolID, permissionCode: permissionCode, riskLevel: riskLevel, configJSON: configJSON, status: status}, nil
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
	query.ToolType = strings.TrimSpace(query.ToolType)
	query.RiskLevel = strings.TrimSpace(query.RiskLevel)
	return query
}

func toolMapDTO(row ToolMapWithEngine) ToolMapDTO {
	return ToolMapDTO{ID: row.ID, ProviderID: row.ProviderID, ProviderName: row.ProviderName, EngineType: row.EngineType, AppID: row.AppID, Name: row.Name, Code: row.Code, ToolType: row.ToolType, ToolTypeName: toolTypeLabels[row.ToolType], EngineToolID: row.EngineToolID, PermissionCode: row.PermissionCode, RiskLevel: row.RiskLevel, RiskLevelName: riskLevelLabels[row.RiskLevel], ConfigJSON: rawJSON(row.ConfigJSON), Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func normalizeRawJSON(value json.RawMessage) (string, *apperror.Error) {
	if len(value) == 0 {
		return "{}", nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return "{}", nil
	}
	if !json.Valid([]byte(trimmed)) {
		return "", apperror.BadRequest("工具配置不是合法JSON")
	}
	return trimmed, nil
}

func rawJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(value)
}

func ensureExists(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具映射失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI工具映射不存在")
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI工具映射仓储未配置")
	}
	return s.repository, nil
}

func stringOptions(values []string, labels map[string]string) []dict.Option[string] {
	options := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, dict.Option[string]{Label: labels[value], Value: value})
	}
	return options
}

func isToolType(value string) bool  { _, ok := toolTypeLabels[value]; return ok }
func isRiskLevel(value string) bool { _, ok := riskLevelLabels[value]; return ok }

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
