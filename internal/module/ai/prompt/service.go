package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
)

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{CommonStatusArr: dict.CommonStatusOptions()}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if !isStatusFilter(query.Status) {
		return nil, apperror.BadRequestKey("ai.prompt.status.invalid", nil, "提示词状态无效")
	}
	rows, total, err := s.repo().List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "ai.prompt.query_failed", nil, "查询AI提示词失败", err)
	}
	query = normalizeListQuery(query)
	return &ListResponse{List: items(rows), Page: page(query.CurrentPage, query.PageSize, total)}, nil
}

func (s *Service) PublicList(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	query.Status = StatusEnabled
	query.IsDel = IsDelActive
	return s.List(ctx, query)
}

func (s *Service) Detail(ctx context.Context, id int64) (*Item, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequestKey("ai.prompt.id.invalid", nil, "提示词ID无效")
	}
	row, err := s.repo().Detail(ctx, id)
	if err != nil {
		return nil, mapRepositoryError(err, "ai.prompt.not_found", "AI提示词不存在", "ai.prompt.query_failed", "查询AI提示词失败")
	}
	if row == nil {
		return nil, apperror.NotFoundKey("ai.prompt.not_found", nil, "AI提示词不存在")
	}
	item := item(*row)
	return &item, nil
}

func (s *Service) Create(ctx context.Context, input Input) (int64, *apperror.Error) {
	input = normalizeInput(input)
	if input.Slug == "" || input.Title == "" || input.Prompt == "" || !isMutationStatus(input.Status) || !json.Valid([]byte(input.TagsJSON)) {
		return 0, apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词参数错误")
	}
	id, err := s.repo().Create(ctx, promptFromInput(input))
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "ai.prompt.create_failed", nil, "创建AI提示词失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input Input) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.prompt.id.invalid", nil, "提示词ID无效")
	}
	input = normalizeInput(input)
	if input.Slug == "" || input.Title == "" || input.Prompt == "" || !isMutationStatus(input.Status) || !json.Valid([]byte(input.TagsJSON)) {
		return apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词参数错误")
	}
	if err := s.repo().Update(ctx, id, promptFromInput(input)); err != nil {
		return mapRepositoryError(err, "ai.prompt.not_found", "AI提示词不存在", "ai.prompt.update_failed", "更新AI提示词失败")
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.prompt.id.invalid", nil, "提示词ID无效")
	}
	if !isStrictStatus(status) {
		return apperror.BadRequestKey("ai.prompt.status.invalid", nil, "提示词状态无效")
	}
	if err := s.repo().ChangeStatus(ctx, id, status); err != nil {
		return mapRepositoryError(err, "ai.prompt.not_found", "AI提示词不存在", "ai.prompt.status_failed", "修改AI提示词状态失败")
	}
	return nil
}

func (s *Service) DeleteOne(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.prompt.id.invalid", nil, "提示词ID无效")
	}
	if err := s.repo().SoftDelete(ctx, id); err != nil {
		return mapRepositoryError(err, "ai.prompt.not_found", "AI提示词不存在", "ai.prompt.delete_failed", "删除AI提示词失败")
	}
	return nil
}

func (s *Service) DeleteBatch(ctx context.Context, ids []int64) *apperror.Error {
	if !validIDs(ids) {
		return apperror.BadRequestKey("ai.prompt.ids.invalid", nil, "提示词ID列表无效")
	}
	if err := s.repo().SoftDeleteBatch(ctx, ids); err != nil {
		return mapRepositoryError(err, "ai.prompt.not_found", "AI提示词不存在", "ai.prompt.delete_batch_failed", "批量删除AI提示词失败")
	}
	return nil
}

func (s *Service) repo() Repository {
	if s == nil || s.repository == nil {
		return failingRepository{}
	}
	return s.repository
}

type failingRepository struct{}

func (failingRepository) List(ctx context.Context, query ListQuery) ([]Prompt, int64, error) {
	return nil, 0, ErrRepositoryNotConfigured
}
func (failingRepository) Detail(ctx context.Context, id int64) (*Prompt, error) {
	return nil, ErrRepositoryNotConfigured
}
func (failingRepository) Create(ctx context.Context, row Prompt) (int64, error) {
	return 0, ErrRepositoryNotConfigured
}
func (failingRepository) Update(ctx context.Context, id int64, row Prompt) error {
	return ErrRepositoryNotConfigured
}
func (failingRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	return ErrRepositoryNotConfigured
}
func (failingRepository) SoftDelete(ctx context.Context, id int64) error {
	return ErrRepositoryNotConfigured
}
func (failingRepository) SoftDeleteBatch(ctx context.Context, ids []int64) error {
	return ErrRepositoryNotConfigured
}

func normalizeInput(input Input) Input {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.TagsJSON = normalizeTagsJSON(input.TagsJSON)
	return input
}

func normalizeTagsJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "[]"
	}
	return value
}

func normalizeStatus(status int) int {
	if status == StatusDisabled {
		return StatusDisabled
	}
	return StatusEnabled
}

func isStatusFilter(status int) bool {
	return status == 0 || isStrictStatus(status)
}

func isMutationStatus(status int) bool {
	return status == 0 || isStrictStatus(status)
}

func isStrictStatus(status int) bool {
	return status == StatusEnabled || status == StatusDisabled
}

func promptFromInput(input Input) Prompt {
	return Prompt{
		Slug:      input.Slug,
		Category:  input.Category,
		Title:     input.Title,
		CoverURL:  input.CoverURL,
		Prompt:    input.Prompt,
		Preview:   input.Preview,
		TagsJSON:  input.TagsJSON,
		SourceURL: input.SourceURL,
		Status:    normalizeStatus(input.Status),
		IsDel:     IsDelActive,
	}
}

func items(rows []Prompt) []Item {
	result := make([]Item, 0, len(rows))
	for _, r := range rows {
		result = append(result, item(r))
	}
	return result
}

func item(r Prompt) Item {
	return Item{ID: r.ID, Slug: r.Slug, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Prompt: r.Prompt, Preview: r.Preview, TagsJSON: r.TagsJSON, SourceURL: r.SourceURL, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)}
}

func validIDs(ids []int64) bool {
	if len(ids) == 0 {
		return false
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return false
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func mapRepositoryError(err error, notFoundKey, notFoundFallback, internalKey, internalFallback string) *apperror.Error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFoundKey(notFoundKey, nil, notFoundFallback)
	}
	return apperror.WrapKey(apperror.CodeInternal, 500, internalKey, nil, internalFallback, err)
}

func page(current, size int, total int64) Page {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	totalPage := int64(0)
	if size > 0 {
		totalPage = (total + int64(size) - 1) / int64(size)
	}
	return Page{CurrentPage: current, PageSize: size, Total: total, TotalPage: int(totalPage)}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
