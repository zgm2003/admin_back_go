package aiengine

import (
	"context"
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

var engineTypeLabels = map[string]string{
	"dify":    "Dify",
	"direct":  "Direct",
	"eino":    "Eino",
	"ragflow": "RAGFlow",
}

var healthStatusOptions = []dict.Option[string]{
	{Label: "未知", Value: "unknown"},
	{Label: "正常", Value: "ok"},
	{Label: "失败", Value: "failed"},
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
	return &InitResponse{Dict: InitDict{EngineTypeArr: engineTypeOptions(), CommonStatusArr: dict.CommonStatusOptions(), HealthStatusArr: healthStatusOptions}}, nil
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
		list = append(list, connectionDTO(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
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
	exists, err := repo.ExistsByTypeName(ctx, row.EngineType, row.Name, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI供应商失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("该引擎类型下已存在同名供应商")
	}
	if strings.TrimSpace(input.APIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密AI供应商API Key失败", err)
		}
		row.APIKeyEnc = ciphertext
		row.APIKeyHint = secretbox.Hint(strings.TrimSpace(input.APIKey))
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI供应商失败", err)
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
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByTypeName(ctx, strings.TrimSpace(input.EngineType), strings.TrimSpace(input.Name), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI供应商失败", err)
	}
	if exists {
		return apperror.BadRequest("该引擎类型下已存在同名供应商")
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
	tester := s.tester
	if tester == nil {
		tester = unsupportedTester{}
	}
	result, testErr := tester.TestConnection(ctx, platformai.TestConnectionInput{EngineType: platformai.EngineType(row.EngineType), BaseURL: row.BaseURL, APIKey: apiKey, TimeoutMs: 10000})
	health := "ok"
	if testErr != nil || result == nil || !result.OK {
		health = "failed"
	}
	fields := map[string]any{"health_status": health, "last_checked_at": time.Now()}
	if err := repo.Update(ctx, id, fields); err != nil {
		return result, apperror.Wrap(apperror.CodeInternal, 500, "更新AI供应商健康状态失败", err)
	}
	if testErr != nil {
		return result, apperror.Wrap(apperror.CodeInternal, 500, "测试AI供应商连接失败", testErr)
	}
	return result, nil
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

func normalizeCreateInput(input CreateInput) (Connection, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.EngineType, input.BaseURL, input.WorkspaceID, input.Status)
	if appErr != nil {
		return Connection{}, appErr
	}
	return Connection{Name: fields.name, EngineType: fields.engineType, BaseURL: fields.baseURL, WorkspaceID: fields.workspaceID, Status: fields.status, HealthStatus: "unknown", IsDel: enum.CommonNo}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.EngineType, input.BaseURL, input.WorkspaceID, input.Status)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{"name": fields.name, "engine_type": fields.engineType, "base_url": fields.baseURL, "workspace_id": fields.workspaceID, "status": fields.status}, nil
}

type normalizedFields struct {
	name, engineType, baseURL, workspaceID string
	status                                 int
}

func normalizeMutationFields(name, engineType, baseURL, workspaceID string, status int) (normalizedFields, *apperror.Error) {
	name = strings.TrimSpace(name)
	engineType = strings.TrimSpace(engineType)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	workspaceID = strings.TrimSpace(workspaceID)
	if name == "" {
		return normalizedFields{}, apperror.BadRequest("供应商名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedFields{}, apperror.BadRequest("供应商名称不能超过128个字符")
	}
	if !isEngineType(engineType) {
		return normalizedFields{}, apperror.BadRequest("无效的AI引擎类型")
	}
	if baseURL == "" {
		return normalizedFields{}, apperror.BadRequest("供应商地址不能为空")
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

func connectionDTO(row Connection) ConnectionDTO {
	return ConnectionDTO{
		ID:             row.ID,
		Name:           row.Name,
		EngineType:     row.EngineType,
		EngineTypeName: engineTypeLabels[row.EngineType],
		BaseURL:        row.BaseURL,
		APIKeyMasked:   row.APIKeyHint,
		WorkspaceID:    row.WorkspaceID,
		HealthStatus:   row.HealthStatus,
		LastCheckedAt:  formatPtrTime(row.LastCheckedAt),
		Status:         row.Status,
		StatusName:     statusText(row.Status),
		CreatedAt:      formatTime(row.CreatedAt),
		UpdatedAt:      formatTime(row.UpdatedAt),
	}
}

func engineTypeOptions() []dict.Option[string] {
	values := []string{"dify", "direct", "eino", "ragflow"}
	options := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, dict.Option[string]{Label: engineTypeLabels[value], Value: value})
	}
	return options
}

func isEngineType(value string) bool { _, ok := engineTypeLabels[value]; return ok }

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
