package aitool

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

var toolCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		AIExecutorTypeArr: dict.AIExecutorTypeOptions(),
		CommonStatusArr:   dict.CommonStatusOptions(),
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, toolItem(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByCode(ctx, row.Code, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI工具失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("工具编码已存在")
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI工具失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI工具ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI工具不存在")
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByCode(ctx, strings.TrimSpace(input.Code), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI工具失败", err)
	}
	if exists {
		return apperror.BadRequest("工具编码已存在")
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI工具失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI工具ID")
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI工具不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI工具状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI工具ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI工具不存在")
	}
	bound, err := repo.HasActiveBindings(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI工具绑定失败", err)
	}
	if bound {
		return apperror.BadRequest("工具已被智能体绑定，请先解绑再删除")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI工具失败", err)
	}
	return nil
}

func (s *Service) AgentOptions(ctx context.Context, agentID int64) (*AgentToolsResponse, *apperror.Error) {
	if agentID < 0 {
		return nil, apperror.BadRequest("无效的智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ActiveOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具选项失败", err)
	}
	options := make([]ToolOption, 0, len(rows))
	activeIDs := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row.Code == retiredCineToolCode {
			continue
		}
		options = append(options, ToolOption{Value: row.ID, Label: row.Name, Code: row.Code})
		activeIDs[row.ID] = struct{}{}
	}

	bound := make([]int64, 0)
	if agentID > 0 {
		ids, err := repo.BoundToolIDs(ctx, agentID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询智能体工具绑定失败", err)
		}
		bound = filterIDs(ids, activeIDs)
	}
	return &AgentToolsResponse{BoundToolIDs: bound, AllTools: options}, nil
}

func (s *Service) SyncAgentBindings(ctx context.Context, agentID int64, toolIDs []int64) *apperror.Error {
	if agentID <= 0 {
		return apperror.BadRequest("无效的智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	normalized := normalizeIDs(toolIDs)
	if err := repo.SyncBindings(ctx, agentID, normalized); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "同步智能体工具绑定失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI工具仓储未配置")
	}
	return s.repository, nil
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
	return query
}

func normalizeCreateInput(input CreateInput) (Tool, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.Code, input.Description, input.SchemaJSON, input.ExecutorType, input.ExecutorConfig, input.Status)
	if appErr != nil {
		return Tool{}, appErr
	}
	return Tool{
		Name: fields.name, Code: fields.code, Description: fields.description, SchemaJSON: fields.schemaJSON,
		ExecutorType: fields.executorType, ExecutorConfig: fields.executorConfig, Status: fields.status, IsDel: enum.CommonNo,
	}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.Code, input.Description, input.SchemaJSON, input.ExecutorType, input.ExecutorConfig, input.Status)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{
		"name":            fields.name,
		"code":            fields.code,
		"description":     fields.description,
		"schema_json":     fields.schemaJSON,
		"executor_type":   fields.executorType,
		"executor_config": fields.executorConfig,
		"status":          fields.status,
	}, nil
}

type normalizedFields struct {
	name           string
	code           string
	description    string
	schemaJSON     *string
	executorType   int
	executorConfig *string
	status         int
}

func normalizeMutationFields(name string, code string, description string, schema JSONObject, executorType int, executorConfig JSONObject, status int) (normalizedFields, *apperror.Error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	description = strings.TrimSpace(description)

	if name == "" {
		return normalizedFields{}, apperror.BadRequest("工具名称不能为空")
	}
	if len([]rune(name)) > 50 {
		return normalizedFields{}, apperror.BadRequest("工具名称不能超过50个字符")
	}
	if !toolCodePattern.MatchString(code) {
		return normalizedFields{}, apperror.BadRequest("工具编码格式错误")
	}
	if len([]rune(description)) > 255 {
		return normalizedFields{}, apperror.BadRequest("描述不能超过255个字符")
	}
	if !enum.IsAIExecutorType(executorType) {
		return normalizedFields{}, apperror.BadRequest("无效的执行器类型")
	}
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	if appErr := validateExecutorConfig(executorType, executorConfig); appErr != nil {
		return normalizedFields{}, appErr
	}
	schemaJSON, err := encodeJSONObject(schema)
	if err != nil {
		return normalizedFields{}, apperror.BadRequest("参数Schema必须是JSON对象")
	}
	configJSON, err := encodeJSONObject(executorConfig)
	if err != nil {
		return normalizedFields{}, apperror.BadRequest("执行器配置必须是JSON对象")
	}
	return normalizedFields{name: name, code: code, description: description, schemaJSON: schemaJSON, executorType: executorType, executorConfig: configJSON, status: status}, nil
}

func validateExecutorConfig(executorType int, config JSONObject) *apperror.Error {
	if executorType == enum.AIExecutorHTTPWhitelist {
		url, _ := config["url"].(string)
		if !strings.HasPrefix(strings.TrimSpace(url), "https://") {
			return apperror.BadRequest("HTTP白名单执行器的 URL 必须以 https:// 开头")
		}
	}
	if executorType == enum.AIExecutorSQLReadonly {
		sql, _ := config["sql"].(string)
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
			return apperror.BadRequest("只读SQL执行器的 SQL 必须以 SELECT 开头")
		}
	}
	return nil
}

func encodeJSONObject(value JSONObject) (*string, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	encoded := string(data)
	return &encoded, nil
}

func decodeJSONObject(value *string) JSONObject {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	var parsed JSONObject
	if err := json.Unmarshal([]byte(*value), &parsed); err != nil {
		return nil
	}
	return parsed
}

func toolItem(row Tool) ListItem {
	return ListItem{
		ID: row.ID, Name: row.Name, Code: row.Code, Description: row.Description, SchemaJSON: decodeJSONObject(row.SchemaJSON),
		ExecutorType: row.ExecutorType, ExecutorName: executorName(row.ExecutorType), ExecutorConfig: decodeJSONObject(row.ExecutorConfig),
		Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func executorName(value int) string {
	return enum.AIExecutorTypeLabels[value]
}

func statusText(value int) string {
	for _, item := range dict.CommonStatusOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return ""
}

func normalizeIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	ids := make([]int64, 0, len(values))
	for _, id := range values {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func filterIDs(values []int64, allowed map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for _, id := range normalizeIDs(values) {
		if _, ok := allowed[id]; ok {
			result = append(result, id)
		}
	}
	return result
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
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
