package asset

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
	return &PageInitResponse{
		CommonStatusArr: dict.CommonStatusOptions(),
		AIAssetTypeArr: []dict.Option[string]{
			{Label: "文本", Value: AssetTypeText},
			{Label: "图片", Value: AssetTypeImage},
			{Label: "视频", Value: AssetTypeVideo},
		},
	}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if !isStatusFilter(query.Status) {
		return nil, apperror.BadRequestKey("ai.asset.status.invalid", nil, "素材状态无效")
	}
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

func (s *Service) Detail(ctx context.Context, id int64) (*Item, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效")
	}
	row, err := s.repo().Detail(ctx, id)
	if err != nil {
		return nil, mapRepositoryError(err, "ai.asset.not_found", "AI素材不存在", "ai.asset.query_failed", "查询AI素材失败")
	}
	if row == nil {
		return nil, apperror.NotFoundKey("ai.asset.not_found", nil, "AI素材不存在")
	}
	item := item(*row)
	return &item, nil
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
		return mapRepositoryError(err, "ai.asset.not_found", "AI素材不存在", "ai.asset.update_failed", "更新AI素材失败")
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	return s.DeleteOne(ctx, id)
}

func (s *Service) DeleteOne(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效")
	}
	if err := s.repo().SoftDelete(ctx, id); err != nil {
		return mapRepositoryError(err, "ai.asset.not_found", "AI素材不存在", "ai.asset.delete_failed", "删除AI素材失败")
	}
	return nil
}

func (s *Service) DeleteBatch(ctx context.Context, ids []int64) *apperror.Error {
	if !validIDs(ids) {
		return apperror.BadRequestKey("ai.asset.ids.invalid", nil, "素材ID列表无效")
	}
	if err := s.repo().SoftDeleteBatch(ctx, ids); err != nil {
		return mapRepositoryError(err, "ai.asset.not_found", "AI素材不存在", "ai.asset.delete_batch_failed", "批量删除AI素材失败")
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
func (failingRepository) Detail(ctx context.Context, id int64) (*Asset, error) {
	return nil, ErrRepositoryNotConfigured
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
func (failingRepository) SoftDeleteBatch(ctx context.Context, ids []int64) error {
	return ErrRepositoryNotConfigured
}

func normalizeInput(input Input) Input {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Type = strings.TrimSpace(input.Type)
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	input.TagsJSON = normalizeTagsJSON(input.TagsJSON)
	return input
}

func validInput(input Input) bool {
	return input.Slug != "" && input.Title != "" && isAssetType(input.Type) && isAssetStatus(input.Status) && json.Valid([]byte(input.TagsJSON))
}

func normalizeTagsJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "[]"
	}
	return value
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

func isStatusFilter(status int) bool {
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
		result = append(result, item(r))
	}
	return result
}

func item(r Asset) Item {
	return Item{ID: r.ID, Slug: r.Slug, Type: r.Type, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Description: r.Description, Content: r.Content, URL: r.URL, TagsJSON: r.TagsJSON, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)}
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
