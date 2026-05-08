package aimodel

import (
	"context"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
	secretbox  secretbox.Box
}

func NewService(repository Repository, box secretbox.Box) *Service {
	return &Service{repository: repository, secretbox: box}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		AIDriverArr:     dict.AIDriverOptions(),
		CommonStatusArr: dict.CommonStatusOptions(),
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI模型失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, modelItem(row))
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
	exists, err := repo.ExistsByDriverName(ctx, row.Driver, row.Name, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI模型失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("该驱动下已存在同名模型")
	}
	if strings.TrimSpace(input.APIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密AI模型API Key失败", err)
		}
		row.APIKeyEnc = ciphertext
		row.APIKeyHint = secretbox.Hint(strings.TrimSpace(input.APIKey))
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI模型失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI模型ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI模型失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI模型不存在")
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByDriverName(ctx, strings.TrimSpace(input.Driver), strings.TrimSpace(input.Name), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI模型失败", err)
	}
	if exists {
		return apperror.BadRequest("该驱动下已存在同名模型")
	}
	if strings.TrimSpace(input.APIKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密AI模型API Key失败", err)
		}
		fields["api_key_enc"] = ciphertext
		fields["api_key_hint"] = secretbox.Hint(strings.TrimSpace(input.APIKey))
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI模型失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI模型ID")
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
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI模型失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI模型不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI模型状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI模型ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI模型失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI模型不存在")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI模型失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI模型仓储未配置")
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
	query.Driver = strings.TrimSpace(query.Driver)
	return query
}

func normalizeCreateInput(input CreateInput) (Model, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.Driver, input.ModelCode, input.Endpoint, input.Status)
	if appErr != nil {
		return Model{}, appErr
	}
	return Model{
		Name:      fields.name,
		Driver:    fields.driver,
		ModelCode: fields.modelCode,
		Endpoint:  fields.endpoint,
		Status:    fields.status,
		IsDel:     enum.CommonNo,
	}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.Driver, input.ModelCode, input.Endpoint, input.Status)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{
		"name":       fields.name,
		"driver":     fields.driver,
		"model_code": fields.modelCode,
		"endpoint":   fields.endpoint,
		"status":     fields.status,
	}, nil
}

type normalizedFields struct {
	name      string
	driver    string
	modelCode string
	endpoint  string
	status    int
}

func normalizeMutationFields(name string, driver string, modelCode string, endpoint string, status int) (normalizedFields, *apperror.Error) {
	name = strings.TrimSpace(name)
	driver = strings.TrimSpace(driver)
	modelCode = strings.TrimSpace(modelCode)
	endpoint = strings.TrimSpace(endpoint)

	if name == "" {
		return normalizedFields{}, apperror.BadRequest("模型名称不能为空")
	}
	if len([]rune(name)) > 50 {
		return normalizedFields{}, apperror.BadRequest("模型名称不能超过50个字符")
	}
	if !enum.IsAIDriver(driver) {
		return normalizedFields{}, apperror.BadRequest("无效的AI驱动")
	}
	if modelCode == "" {
		return normalizedFields{}, apperror.BadRequest("模型标识不能为空")
	}
	if len([]rune(modelCode)) > 80 {
		return normalizedFields{}, apperror.BadRequest("模型标识不能超过80个字符")
	}
	if len([]rune(endpoint)) > 255 {
		return normalizedFields{}, apperror.BadRequest("接口地址不能超过255个字符")
	}
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	return normalizedFields{name: name, driver: driver, modelCode: modelCode, endpoint: endpoint, status: status}, nil
}

func modelItem(row Model) ListItem {
	return ListItem{
		ID: row.ID, Name: row.Name, Driver: row.Driver, DriverName: driverName(row.Driver), ModelCode: row.ModelCode,
		Endpoint: row.Endpoint, APIKeyHint: row.APIKeyHint, Status: row.Status, StatusName: statusText(row.Status),
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func driverName(value string) string {
	if label := enum.AIDriverLabels[value]; label != "" {
		return label
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
