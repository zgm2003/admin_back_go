package prompt

import (
	"context"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
)

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
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

func (s *Service) Create(ctx context.Context, input Input) (int64, *apperror.Error) {
	input = normalizeInput(input)
	if input.Slug == "" || input.Title == "" || input.Prompt == "" {
		return 0, apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词参数错误")
	}
	row := Prompt{
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
	id, err := s.repo().Create(ctx, row)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "ai.prompt.create_failed", nil, "创建AI提示词失败", err)
	}
	return id, nil
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
func (failingRepository) Create(ctx context.Context, row Prompt) (int64, error) {
	return 0, ErrRepositoryNotConfigured
}
func (failingRepository) SoftDelete(ctx context.Context, id int64) error {
	return ErrRepositoryNotConfigured
}

func normalizeInput(input Input) Input {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	input.Prompt = strings.TrimSpace(input.Prompt)
	return input
}

func normalizeStatus(status int) int {
	if status == StatusDisabled {
		return StatusDisabled
	}
	return StatusEnabled
}

func items(rows []Prompt) []Item {
	result := make([]Item, 0, len(rows))
	for _, r := range rows {
		result = append(result, Item{ID: r.ID, Slug: r.Slug, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Prompt: r.Prompt, Preview: r.Preview, TagsJSON: r.TagsJSON, SourceURL: r.SourceURL, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)})
	}
	return result
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
