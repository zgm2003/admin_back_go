package asset

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
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "ai.asset.query_failed", nil, "查询AI素材失败", err)
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
	if !validInput(input) {
		return 0, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误")
	}
	id, err := s.repo().Create(ctx, assetFromInput(input))
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "ai.asset.create_failed", nil, "创建AI素材失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input Input) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效")
	}
	input = normalizeInput(input)
	if !validInput(input) {
		return apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误")
	}
	if err := s.repo().Update(ctx, id, assetFromInput(input)); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "ai.asset.update_failed", nil, "更新AI素材失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效")
	}
	if err := s.repo().SoftDelete(ctx, id); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "ai.asset.delete_failed", nil, "删除AI素材失败", err)
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

func (failingRepository) List(ctx context.Context, query ListQuery) ([]Asset, int64, error) {
	return nil, 0, ErrRepositoryNotConfigured
}
func (failingRepository) Create(ctx context.Context, row Asset) (int64, error) {
	return 0, ErrRepositoryNotConfigured
}
func (failingRepository) Update(ctx context.Context, id int64, row Asset) error {
	return ErrRepositoryNotConfigured
}
func (failingRepository) SoftDelete(ctx context.Context, id int64) error {
	return ErrRepositoryNotConfigured
}

func normalizeInput(input Input) Input {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Type = strings.TrimSpace(input.Type)
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	return input
}

func validInput(input Input) bool {
	return input.Slug != "" && input.Title != "" && isAssetType(input.Type) && isAssetStatus(input.Status)
}

func isAssetType(value string) bool {
	return value == AssetTypeText || value == AssetTypeImage || value == AssetTypeVideo
}

func normalizeStatus(status int) int {
	if status == StatusDisabled {
		return StatusDisabled
	}
	return StatusEnabled
}

func isAssetStatus(status int) bool {
	return status == 0 || status == StatusEnabled || status == StatusDisabled
}

func assetFromInput(input Input) Asset {
	return Asset{
		Slug:        input.Slug,
		Type:        input.Type,
		Category:    input.Category,
		Title:       input.Title,
		CoverURL:    input.CoverURL,
		Description: input.Description,
		Content:     input.Content,
		URL:         input.URL,
		TagsJSON:    input.TagsJSON,
		Status:      normalizeStatus(input.Status),
		IsDel:       IsDelActive,
	}
}

func items(rows []Asset) []Item {
	result := make([]Item, 0, len(rows))
	for _, r := range rows {
		result = append(result, Item{ID: r.ID, Slug: r.Slug, Type: r.Type, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Description: r.Description, Content: r.Content, URL: r.URL, TagsJSON: r.TagsJSON, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)})
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
