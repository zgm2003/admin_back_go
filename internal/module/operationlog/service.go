package operationlog

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("操作日志仓储未配置")
	}

	normalized, appErr := normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}

	rows, total, err := s.repository.List(ctx, normalized)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询操作日志失败", err)
	}

	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, ListItem{
			ID:           row.ID,
			UserName:     row.UserName,
			UserEmail:    row.UserEmail,
			Action:       row.Action,
			RequestData:  row.RequestData,
			ResponseData: row.ResponseData,
			IsSuccess:    row.IsSuccess,
			CreatedAt:    formatTime(row.CreatedAt),
		})
	}

	return &ListResponse{
		List: list,
		Page: Page{
			PageSize:    normalized.PageSize,
			CurrentPage: normalized.CurrentPage,
			TotalPage:   totalPage(total, normalized.PageSize),
			Total:       total,
		},
	}, nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	if s == nil || s.repository == nil {
		return apperror.Internal("操作日志仓储未配置")
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的操作日志")
	}
	if err := s.repository.Delete(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除操作日志失败", err)
	}
	return nil
}

func NewRecorder(repository Repository) middleware.OperationRecorder {
	return func(ctx context.Context, input middleware.OperationInput) error {
		if repository == nil {
			return ErrRepositoryNotConfigured
		}

		requestData, err := json.Marshal(buildLogRequestPayload(input))
		if err != nil {
			return err
		}

		responseData, err := json.Marshal(buildLogResponsePayload(input))
		if err != nil {
			return err
		}

		isSuccess := CommonNo
		if input.Success {
			isSuccess = CommonYes
		}

		action := input.Title
		if action == "" {
			action = input.Module + "." + input.Action
		}

		return repository.Create(ctx, Log{
			UserID:       input.UserID,
			Action:       action,
			RequestData:  string(requestData),
			ResponseData: string(responseData),
			IsDel:        CommonNo,
			IsSuccess:    isSuccess,
		})
	}
}

func buildLogRequestPayload(input middleware.OperationInput) map[string]any {
	payload := map[string]any{
		"method":     input.Method,
		"path":       input.Path,
		"module":     input.Module,
		"action":     input.Action,
		"request_id": input.RequestID,
		"client_ip":  input.ClientIP,
		"session_id": input.SessionID,
		"platform":   input.Platform,
		"latency_ms": input.LatencyMs,
	}
	if input.RequestPayload != nil {
		payload["payload"] = sanitizeForLog(input.RequestPayload, true)
	}
	return payload
}

func buildLogResponsePayload(input middleware.OperationInput) map[string]any {
	payload := map[string]any{
		"status":  input.Status,
		"success": input.Success,
	}
	if input.ResponsePayload != nil {
		payload["payload"] = sanitizeForLog(input.ResponsePayload, false)
	}
	return payload
}

func sanitizeForLog(value any, maskGenericCode bool) any {
	return sanitizeForLogValue(value, maskGenericCode, false)
}

func sanitizeForLogValue(value any, maskGenericCode bool, maskAllLeaves bool) any {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, nested := range current {
			lowerKey := strings.ToLower(key)
			if lowerKey == "captcha_answer" {
				result[key] = sanitizeForLogValue(nested, maskGenericCode, true)
				continue
			}
			if shouldMaskField(lowerKey, maskGenericCode) {
				result[key] = "******"
				continue
			}
			result[key] = sanitizeForLogValue(nested, maskGenericCode, maskAllLeaves)
		}
		return result
	case []any:
		result := make([]any, 0, len(current))
		for _, nested := range current {
			result = append(result, sanitizeForLogValue(nested, maskGenericCode, maskAllLeaves))
		}
		return result
	default:
		if maskAllLeaves {
			return "******"
		}
		return value
	}
}

func shouldMaskField(field string, maskGenericCode bool) bool {
	if maskGenericCode && field == "code" {
		return true
	}
	switch field {
	case "password", "old_password", "new_password", "newpassword", "confirm_password",
		"refresh_token", "access_token", "token", "authorization", "secret", "secret_id",
		"secret_id_enc", "secret_key", "secret_key_enc", "api_key", "api_key_enc",
		"engine_app_api_key", "engine_app_api_key_enc", "captcha", "captcha_code",
		"sms_code", "email_code", "verification_code", "app_private_key", "app_private_key_enc":
		return true
	default:
		return false
	}
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		return query, apperror.BadRequest("当前页无效")
	}
	if query.PageSize < 1 || query.PageSize > 50 {
		return query, apperror.BadRequest("每页数量无效")
	}
	query.Action = strings.TrimSpace(query.Action)
	query.DateRange = normalizeDateRange(query.DateRange)
	return query, nil
}

func normalizeDateRange(values []string) []string {
	if len(values) < 2 {
		return nil
	}
	start := strings.TrimSpace(values[0])
	end := strings.TrimSpace(values[1])
	if start == "" || end == "" {
		return nil
	}
	return []string{start, end}
}

func normalizeIDs(ids []int64) []int64 {
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

func sortStrings(values []string) []string {
	sort.Strings(values)
	return values
}
