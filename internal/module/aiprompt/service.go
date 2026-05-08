package aiprompt

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
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

func (s *Service) List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(userID, query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI提示词失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, promptItem(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Detail(ctx context.Context, userID int64, id int64) (*DetailResponse, *apperror.Error) {
	row, appErr := s.requirePromptForUser(ctx, userID, id, "无权访问")
	if appErr != nil {
		return nil, appErr
	}
	return &DetailResponse{
		ID: row.ID, Title: row.Title, Content: row.Content, Category: row.Category,
		Tags: decodeTags(row.Tags), Variables: decodeVariables(row.Variables),
		IsFavorite: row.IsFavorite, UseCount: row.UseCount, Sort: row.Sort,
	}, nil
}

func (s *Service) Create(ctx context.Context, userID int64, input CreateInput) (int64, *apperror.Error) {
	if userID <= 0 {
		return 0, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeCreateInput(userID, input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI提示词失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, userID int64, id int64, input UpdateInput) *apperror.Error {
	_, appErr := s.requireOwnedPrompt(ctx, userID, id)
	if appErr != nil {
		return appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	if len(fields) == 0 {
		return nil
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI提示词失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID int64, id int64) *apperror.Error {
	_, appErr := s.requireOwnedPrompt(ctx, userID, id)
	if appErr != nil {
		return appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI提示词失败", err)
	}
	return nil
}

func (s *Service) ToggleFavorite(ctx context.Context, userID int64, id int64) (*ToggleFavoriteResponse, *apperror.Error) {
	row, appErr := s.requirePromptForUser(ctx, userID, id, "无权操作")
	if appErr != nil {
		return nil, appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	newValue := enum.CommonNo
	if row.IsFavorite != enum.CommonYes {
		newValue = enum.CommonYes
	}
	if err := repo.Update(ctx, id, map[string]any{"is_favorite": newValue}); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "切换AI提示词收藏状态失败", err)
	}
	return &ToggleFavoriteResponse{IsFavorite: newValue}, nil
}

func (s *Service) Use(ctx context.Context, userID int64, id int64) (*UseResponse, *apperror.Error) {
	row, appErr := s.requireOwnedPrompt(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if err := repo.IncrementUseCount(ctx, id); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "递增AI提示词使用次数失败", err)
	}
	return &UseResponse{Content: row.Content}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI提示词仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireOwnedPrompt(ctx context.Context, userID int64, id int64) (*Prompt, *apperror.Error) {
	return s.requirePromptForUser(ctx, userID, id, "无权操作")
}

func (s *Service) requirePromptForUser(ctx context.Context, userID int64, id int64, deniedMessage string) (*Prompt, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI提示词失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI提示词不存在")
	}
	if row.UserID != userID {
		return nil, apperror.Forbidden(deniedMessage)
	}
	return row, nil
}

func normalizeListQuery(userID int64, query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.UserID = userID
	query.Title = strings.TrimSpace(query.Title)
	query.Category = strings.TrimSpace(query.Category)
	return query
}

func normalizeCreateInput(userID int64, input CreateInput) (Prompt, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Title, input.Content, input.Category, input.Tags, input.Variables)
	if appErr != nil {
		return Prompt{}, appErr
	}
	return Prompt{
		UserID: userID, Title: fields.title, Content: fields.content, Category: fields.category,
		Tags: fields.tags, Variables: fields.variables, IsFavorite: enum.CommonNo, UseCount: 0, Sort: 0, IsDel: enum.CommonNo,
	}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Title, input.Content, input.Category, input.Tags, input.Variables)
	if appErr != nil {
		return nil, appErr
	}
	result := map[string]any{}
	if input.Title != "" {
		result["title"] = fields.title
	}
	if input.Content != "" {
		result["content"] = fields.content
	}
	result["category"] = fields.category
	if input.Tags != nil {
		result["tags"] = fields.tags
	}
	if input.Variables != nil {
		result["variables"] = fields.variables
	}
	return result, nil
}

type normalizedFields struct {
	title     string
	content   string
	category  string
	tags      string
	variables *string
}

func normalizeMutationFields(title string, content string, category string, tags []string, variables []string) (normalizedFields, *apperror.Error) {
	title = strings.TrimSpace(title)
	category = strings.TrimSpace(category)
	contentForValidation := strings.TrimSpace(content)

	if title == "" {
		return normalizedFields{}, apperror.BadRequest("标题不能为空")
	}
	if len([]rune(title)) > 100 {
		return normalizedFields{}, apperror.BadRequest("标题不能超过100个字符")
	}
	if contentForValidation == "" {
		return normalizedFields{}, apperror.BadRequest("提示词内容不能为空")
	}
	if len([]rune(category)) > 50 {
		return normalizedFields{}, apperror.BadRequest("分类不能超过50个字符")
	}
	tagsJSON, err := encodeTags(tags)
	if err != nil {
		return normalizedFields{}, apperror.BadRequest("标签格式错误")
	}
	variablesJSON, err := encodeStringArray(variables)
	if err != nil {
		return normalizedFields{}, apperror.BadRequest("变量格式错误")
	}
	return normalizedFields{title: title, content: content, category: category, tags: tagsJSON, variables: variablesJSON}, nil
}

func encodeTags(values []string) (string, error) {
	normalized := normalizeStringSlice(values)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func encodeStringArray(values []string) (*string, error) {
	normalized := normalizeStringSlice(values)
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	encoded := string(data)
	return &encoded, nil
}

func normalizeStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func decodeTags(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}
	}
	var parsed []string
	if err := json.Unmarshal([]byte(value), &parsed); err == nil {
		return normalizeStringSlice(parsed)
	}
	parts := strings.Split(value, ",")
	return normalizeStringSlice(parts)
}

func decodeVariables(value *string) []string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return []string{}
	}
	var raw any
	if err := json.Unmarshal([]byte(*value), &raw); err != nil {
		return []string{}
	}
	switch typed := raw.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			switch value := item.(type) {
			case string:
				value = strings.TrimSpace(value)
				if value != "" {
					result = append(result, value)
				}
			case map[string]any:
				if key, ok := value["key"].(string); ok {
					key = strings.TrimSpace(key)
					if key != "" {
						result = append(result, key)
					}
				}
			}
		}
		return result
	}
	return []string{}
}

func promptItem(row Prompt) ListItem {
	return ListItem{
		ID: row.ID, Title: row.Title, Content: row.Content, Category: row.Category, Tags: decodeTags(row.Tags),
		Variables: decodeVariables(row.Variables), IsFavorite: row.IsFavorite, UseCount: row.UseCount, Sort: row.Sort,
		CreatedAt: formatTime(row.CreatedAt),
	}
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
