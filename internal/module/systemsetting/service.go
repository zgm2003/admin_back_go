package systemsetting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
	shareddict "admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/pagination"
)

const (
	maxKeyLen    = 100
	maxRemarkLen = 255
	timeLayout   = "2006-01-02 15:04:05"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) PageInit(context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		SystemSettingValueTypeArr: shareddict.SystemSettingValueTypeOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, request ListRequest) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := validateListRequest(request); appErr != nil {
		return nil, appErr
	}
	request.Key = strings.TrimSpace(request.Key)

	rows, total, err := repo.List(ctx, request)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}

	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItemFromSetting(row))
	}
	return &ListResponse{
		List: list,
		Page: pagination.Page{
			PageSize:    request.PageSize,
			CurrentPage: request.CurrentPage,
			TotalPage:   totalPage(total, request.PageSize),
			Total:       total,
		},
	}, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	request, appErr = normalizeCreateRequest(request)
	if appErr != nil {
		return 0, appErr
	}
	existing, err := repo.FindByKey(ctx, request.Key)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.key_check_failed", nil, "校验配置 key 失败", err)
	}
	if existing != nil {
		if existing.IsDel != enum.CommonYes {
			return 0, apperror.BadRequestKey("systemsetting.key.duplicate", map[string]any{"key": request.Key}, "配置 key ["+request.Key+"] 已存在")
		}
		restored, err := repo.Restore(ctx, existing.ID, Setting{
			SettingKey: request.Key, SettingValue: request.Value, ValueType: request.Type,
			Remark: request.Remark, Status: enum.CommonYes, IsDel: enum.CommonNo,
		})
		if err != nil {
			return 0, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.restore_failed", nil, "恢复系统设置失败", err)
		}
		if !restored {
			return 0, apperror.BadRequestKey("systemsetting.key.duplicate", map[string]any{"key": request.Key}, "配置 key ["+request.Key+"] 已存在")
		}
		repo.InvalidateCache(ctx, request.Key)
		return existing.ID, nil
	}

	id, err := repo.Create(ctx, Setting{
		SettingKey: request.Key, SettingValue: request.Value, ValueType: request.Type, Remark: request.Remark,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateKey) {
			return 0, apperror.BadRequestKey("systemsetting.key.duplicate", map[string]any{"key": request.Key}, "配置 key ["+request.Key+"] 已存在")
		}
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.create_failed", nil, "新增系统设置失败", err)
	}
	repo.InvalidateCache(ctx, request.Key)
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, request UpdateRequest) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("systemsetting.id.invalid", nil, "无效的配置ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("systemsetting.not_found", nil, "配置项不存在")
	}

	request, appErr = normalizeUpdateRequest(request)
	if appErr != nil {
		return appErr
	}
	if err := repo.Update(ctx, id, map[string]any{
		"setting_value": request.Value,
		"value_type":    request.Type,
		"remark":        request.Remark,
	}); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.update_failed", nil, "更新系统设置失败", err)
	}
	repo.InvalidateCache(ctx, row.SettingKey)
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("systemsetting.id.invalid", nil, "无效的配置ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequestKey("systemsetting.status.invalid", nil, "无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("systemsetting.not_found", nil, "配置项不存在")
	}
	if err := repo.Update(ctx, id, map[string]any{"status": status}); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.status_update_failed", nil, "更新系统设置状态失败", err)
	}
	repo.InvalidateCache(ctx, row.SettingKey)
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequestKey("systemsetting.delete.empty", nil, "请选择要删除的配置")
	}
	rows, err := repo.SettingsByIDs(ctx, ids)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}
	if len(rows) != len(ids) {
		return apperror.BadRequestKey("systemsetting.delete.contains_missing", nil, "包含不存在的配置项")
	}
	if err := repo.Delete(ctx, ids); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.delete_failed", nil, "删除系统设置失败", err)
	}
	for _, id := range ids {
		repo.InvalidateCache(ctx, rows[id].SettingKey)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("systemsetting.repository_missing", nil, "系统设置仓储未配置")
	}
	return s.repository, nil
}

func validateListRequest(request ListRequest) *apperror.Error {
	if request.CurrentPage <= 0 {
		return apperror.BadRequestKey("systemsetting.current_page.invalid", nil, "当前页无效")
	}
	if request.PageSize < enum.PageSizeMin || request.PageSize > enum.PageSizeMax {
		return apperror.BadRequestKey("systemsetting.page_size.invalid", nil, "每页数量无效")
	}
	if request.Status != nil && !enum.IsCommonStatus(*request.Status) {
		return apperror.BadRequestKey("systemsetting.status.invalid", nil, "无效的状态")
	}
	return nil
}

func normalizeCreateRequest(request CreateRequest) (CreateRequest, *apperror.Error) {
	request.Key = strings.TrimSpace(request.Key)
	if request.Key == "" || len([]rune(request.Key)) > maxKeyLen {
		return request, apperror.BadRequestKey("systemsetting.key.invalid", nil, "配置 key 不能为空且不能超过100个字符")
	}
	return normalizeCreateFields(request)
}

func normalizeCreateFields(request CreateRequest) (CreateRequest, *apperror.Error) {
	update, appErr := normalizeUpdateRequest(UpdateRequest{Value: request.Value, Type: request.Type, Remark: request.Remark})
	if appErr != nil {
		return request, appErr
	}
	request.Value = update.Value
	request.Type = update.Type
	request.Remark = update.Remark
	return request, nil
}

func normalizeUpdateRequest(request UpdateRequest) (UpdateRequest, *apperror.Error) {
	request.Remark = strings.TrimSpace(request.Remark)
	if len([]rune(request.Remark)) > maxRemarkLen {
		return request, apperror.BadRequestKey("systemsetting.remark.too_long", nil, "备注不能超过255个字符")
	}
	if !enum.IsSystemSettingValueType(request.Type) {
		return request, apperror.BadRequestKey("systemsetting.value_type.invalid", nil, "无效的配置值类型")
	}
	if appErr := validateTypedValue(request.Type, request.Value); appErr != nil {
		return request, appErr
	}
	return request, nil
}

func validateTypedValue(valueType int, value string) *apperror.Error {
	switch valueType {
	case enum.SystemSettingValueNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return apperror.BadRequestKey("systemsetting.value.number_invalid", nil, "数值类型需为数字")
		}
	case enum.SystemSettingValueBool:
		switch strings.ToLower(value) {
		case "0", "1", "true", "false":
		default:
			return apperror.BadRequestKey("systemsetting.value.bool_invalid", nil, "布尔类型需为 true/false 或 0/1")
		}
	case enum.SystemSettingValueJSON:
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return apperror.BadRequestKey("systemsetting.value.json_invalid", nil, "JSON 类型需为合法 JSON")
		}
		switch decoded.(type) {
		case map[string]any, []any:
			return nil
		default:
			return apperror.BadRequestKey("systemsetting.value.json_object_invalid", nil, "JSON 类型需为合法对象或数组")
		}
	}
	return nil
}

func listItemFromSetting(row Setting) ListItem {
	return ListItem{
		ID: row.ID, SettingKey: row.SettingKey, SettingValue: row.SettingValue,
		ValueType: row.ValueType, ValueTypeName: valueTypeLabel(row.ValueType),
		Remark: row.Remark, Status: row.Status, StatusName: statusLabel(row.Status), IsDel: row.IsDel,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func normalizeIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
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

func statusLabel(status int) string {
	switch status {
	case enum.CommonYes:
		return "启用"
	case enum.CommonNo:
		return "禁用"
	default:
		return ""
	}
}

func valueTypeLabel(valueType int) string {
	for _, item := range shareddict.SystemSettingValueTypeOptions() {
		if item.Value == valueType {
			return item.Label
		}
	}
	return ""
}
