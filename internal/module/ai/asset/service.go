package asset

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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
	if query.UserID == 0 {
		return nil, apperror.BadRequestKey("ai.asset.user.required", nil, "素材归属用户不能为空")
	}
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

func (s *Service) UserList(ctx context.Context, userID uint64, query ListQuery) (*ListResponse, *apperror.Error) {
	query.UserID = userID
	query.Status = StatusEnabled
	query.IsDel = IsDelActive
	return s.List(ctx, query)
}

func (s *Service) Create(ctx context.Context, input Input) (int64, *apperror.Error) {
	input = normalizeInput(input)
	if input.UserID == 0 {
		return 0, apperror.BadRequestKey("ai.asset.user.required", nil, "素材归属用户不能为空")
	}
	if !validInput(input) {
		return 0, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误")
	}
	id, err := s.repo().Create(ctx, assetFromInput(input))
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "ai.asset.create_failed", nil, "创建AI素材失败", err)
	}
	return id, nil
}

func (s *Service) UserCreate(ctx context.Context, userID uint64, input Input) (int64, *apperror.Error) {
	input.UserID = userID
	return s.Create(ctx, input)
}

func (s *Service) Update(ctx context.Context, id int64, input Input) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效")
	}
	input = normalizeInput(input)
	if input.UserID == 0 {
		return apperror.BadRequestKey("ai.asset.user.required", nil, "素材归属用户不能为空")
	}
	if !validInput(input) {
		return apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误")
	}
	if err := s.repo().Update(ctx, id, assetFromInput(input)); err != nil {
		return mapRepositoryError(err, "ai.asset.not_found", "AI素材不存在", "ai.asset.update_failed", "更新AI素材失败")
	}
	return nil
}

func (s *Service) UserUpdate(ctx context.Context, userID uint64, id int64, input Input) *apperror.Error {
	input.UserID = userID
	return s.Update(ctx, id, input)
}

func (s *Service) UserDelete(ctx context.Context, userID uint64, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效")
	}
	if userID == 0 {
		return apperror.BadRequestKey("ai.asset.user.required", nil, "素材归属用户不能为空")
	}
	if err := s.repo().SoftDelete(ctx, id, userID); err != nil {
		return mapRepositoryError(err, "ai.asset.not_found", "AI素材不存在", "ai.asset.delete_failed", "删除AI素材失败")
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
func (failingRepository) SoftDelete(ctx context.Context, id int64, userID uint64) error {
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
	if input.Slug == "" || input.Title == "" || !isAssetType(input.Type) || !isAssetStatus(input.Status) || !json.Valid([]byte(input.TagsJSON)) {
		return false
	}
	if input.Type == AssetTypeText {
		return true
	}
	return validMediaMetadata(input)
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

type mediaMetadata struct {
	StorageKey string   `json:"storageKey"`
	Width      float64  `json:"width"`
	Height     float64  `json:"height"`
	Bytes      float64  `json:"bytes"`
	MimeType   string   `json:"mimeType"`
	Duration   *float64 `json:"duration"`
}

func validMediaMetadata(input Input) bool {
	if strings.TrimSpace(input.URL) == "" {
		return false
	}
	raw := strings.TrimSpace(input.Content)
	if raw == "" {
		return false
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || len(fields) == 0 {
		return false
	}
	for key := range fields {
		if !isAllowedMediaMetadataKey(key) {
			return false
		}
	}
	if rawMetadata, ok := fields["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(rawMetadata, &metadata); err != nil || metadata == nil {
			return false
		}
	}

	var metadata mediaMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return false
	}
	if !validStorageKey(input.Type, metadata.StorageKey) {
		return false
	}
	if !positiveNumber(metadata.Width) || !positiveNumber(metadata.Height) || !positiveNumber(metadata.Bytes) {
		return false
	}
	if metadata.Duration != nil && !positiveNumber(*metadata.Duration) {
		return false
	}
	mimeType := strings.TrimSpace(metadata.MimeType)
	if input.Type == AssetTypeImage {
		return strings.HasPrefix(mimeType, "image/")
	}
	return strings.HasPrefix(mimeType, "video/")
}

func isAllowedMediaMetadataKey(key string) bool {
	switch key {
	case "storageKey", "width", "height", "bytes", "mimeType", "duration", "metadata":
		return true
	default:
		return false
	}
}

func validStorageKey(assetType, value string) bool {
	key := strings.TrimSpace(value)
	if key == "" || strings.HasPrefix(key, "blob:") || strings.HasPrefix(key, "data:") {
		return false
	}
	if isBrowserLocalStorageKey(key) {
		return false
	}
	if !strings.Contains(key, ":") {
		return strings.Contains(key, "/")
	}
	if assetType == AssetTypeImage {
		return strings.HasPrefix(key, "image:")
	}
	return strings.HasPrefix(key, "video:") || strings.HasPrefix(key, "file:") || strings.HasPrefix(key, "media:")
}

func isBrowserLocalStorageKey(key string) bool {
	colon := strings.Index(key, ":")
	if colon <= 0 || strings.Contains(key[colon+1:], "/") {
		return false
	}
	prefix := key[:colon]
	return prefix == "image" || prefix == "video" || prefix == "file" || prefix == "media"
}

func positiveNumber(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func assetFromInput(input Input) Asset {
	return Asset{
		UserID:      input.UserID,
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
	return Item{ID: r.ID, UserID: r.UserID, Slug: r.Slug, Type: r.Type, Category: r.Category, Title: r.Title, CoverURL: r.CoverURL, Description: r.Description, Content: r.Content, URL: r.URL, TagsJSON: r.TagsJSON, Status: r.Status, CreatedAt: formatTime(r.CreatedAt), UpdatedAt: formatTime(r.UpdatedAt)}
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
