package clientversion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

type ManifestPublisher interface {
	Publish(ctx context.Context, platform string, body []byte) error
}

type Service struct {
	repository Repository
	publisher  ManifestPublisher
}

func NewService(repository Repository, publisher ManifestPublisher) *Service {
	return &Service{repository: repository, publisher: publisher}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		ClientVersionPlatformArr: dict.ClientVersionPlatformOptions(),
		CommonYesNoArr:           dict.CommonYesNoOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	if query.Platform != "" && !enum.IsClientPlatform(query.Platform) {
		return nil, apperror.BadRequestKey("clientversion.platform.invalid", nil, "无效的客户端平台")
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.list_failed", nil, "查询版本列表失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row))
	}
	return &ListResponse{List: list, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: total, TotalPage: totalPage(total, query.PageSize)}}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	input, appErr = normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByVersionPlatform(ctx, input.Version, input.Platform, 0)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.unique_check_failed", nil, "校验版本唯一性失败", err)
	}
	if exists {
		return 0, apperror.BadRequestKey("clientversion.version.duplicate", nil, "该平台版本已存在")
	}
	id, err := repo.Create(ctx, Version{
		Version: input.Version, Notes: input.Notes, FileURL: input.FileURL, Signature: input.Signature, Platform: input.Platform,
		FileSize: input.FileSize, IsLatest: enum.CommonNo, ForceUpdate: input.ForceUpdate, IsDel: enum.CommonNo,
	})
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.create_failed", nil, "新增客户端版本失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("clientversion.id.invalid", nil, "无效的版本ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, appErr := getVersion(ctx, repo, id)
	if appErr != nil {
		return appErr
	}
	input, appErr = normalizeUpdateInput(input)
	if appErr != nil {
		return appErr
	}
	if row.Platform != input.Platform {
		return apperror.BadRequestKey("clientversion.platform_immutable", nil, "版本平台不允许修改")
	}
	exists, err := repo.ExistsByVersionPlatform(ctx, input.Version, input.Platform, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.unique_check_failed", nil, "校验版本唯一性失败", err)
	}
	if exists {
		return apperror.BadRequestKey("clientversion.version.duplicate", nil, "该平台版本已存在")
	}
	fields := map[string]any{
		"version":      input.Version,
		"notes":        input.Notes,
		"file_url":     input.FileURL,
		"signature":    input.Signature,
		"file_size":    input.FileSize,
		"force_update": input.ForceUpdate,
	}
	updated := *row
	updated.Version = input.Version
	updated.Notes = input.Notes
	updated.FileURL = input.FileURL
	updated.Signature = input.Signature
	updated.FileSize = input.FileSize
	updated.ForceUpdate = input.ForceUpdate
	if row.IsLatest != enum.CommonYes {
		if err := repo.Update(ctx, id, fields); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.update_failed", nil, "更新客户端版本失败", err)
		}
		return nil
	}
	return s.withPublishTx(ctx, func(txRepo Repository) error {
		if err := txRepo.Update(ctx, id, fields); err != nil {
			return fmt.Errorf("update latest version: %w", err)
		}
		return s.publishManifest(ctx, updated)
	})
}

func (s *Service) SetLatest(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("clientversion.id.invalid", nil, "无效的版本ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, appErr := getVersion(ctx, repo, id)
	if appErr != nil {
		return appErr
	}
	return s.withPublishTx(ctx, func(txRepo Repository) error {
		if err := txRepo.ClearLatestByPlatform(ctx, row.Platform); err != nil {
			return fmt.Errorf("clear latest: %w", err)
		}
		if err := txRepo.SetLatest(ctx, id); err != nil {
			return fmt.Errorf("set latest: %w", err)
		}
		latest := *row
		latest.IsLatest = enum.CommonYes
		return s.publishManifest(ctx, latest)
	})
}

func (s *Service) ForceUpdate(ctx context.Context, id int64, forceUpdate int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("clientversion.id.invalid", nil, "无效的版本ID")
	}
	if !enum.IsCommonYesNo(forceUpdate) {
		return apperror.BadRequestKey("clientversion.force_update.invalid", nil, "无效的强制更新状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, appErr := getVersion(ctx, repo, id)
	if appErr != nil {
		return appErr
	}
	fields := map[string]any{"force_update": forceUpdate}
	if row.IsLatest != enum.CommonYes {
		if err := repo.Update(ctx, id, fields); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.force_update_failed", nil, "更新强制更新状态失败", err)
		}
		return nil
	}
	updated := *row
	updated.ForceUpdate = forceUpdate
	return s.withPublishTx(ctx, func(txRepo Repository) error {
		if err := txRepo.Update(ctx, id, fields); err != nil {
			return fmt.Errorf("update force status: %w", err)
		}
		return s.publishManifest(ctx, updated)
	})
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("clientversion.id.invalid", nil, "无效的版本ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, appErr := getVersion(ctx, repo, id)
	if appErr != nil {
		return appErr
	}
	if row.IsLatest == enum.CommonYes {
		return apperror.BadRequestKey("clientversion.latest_delete_forbidden", nil, "不能删除当前最新版本")
	}
	if err := repo.SoftDelete(ctx, id); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.delete_failed", nil, "删除客户端版本失败", err)
	}
	return nil
}

func (s *Service) UpdateJSON(ctx context.Context, platform string) (any, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = enum.ClientPlatformWindowsX8664
	}
	if !enum.IsClientPlatform(platform) {
		return nil, apperror.BadRequestKey("clientversion.platform.invalid", nil, "无效的客户端平台")
	}
	row, err := repo.Latest(ctx, platform)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.latest_query_failed", nil, "查询最新版本失败", err)
	}
	if row == nil {
		return []any{}, nil
	}
	return manifestPayload(*row), nil
}

func (s *Service) CurrentCheck(ctx context.Context, query CurrentCheckQuery) (*CurrentCheckResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Version = strings.TrimSpace(query.Version)
	query.Platform = strings.TrimSpace(query.Platform)
	if query.Platform == "" {
		query.Platform = enum.ClientPlatformWindowsX8664
	}
	if query.Version == "" {
		return nil, apperror.BadRequestKey("clientversion.current_version.required", nil, "版本号不能为空")
	}
	if !enum.IsClientPlatform(query.Platform) {
		return nil, apperror.BadRequestKey("clientversion.platform.invalid", nil, "无效的客户端平台")
	}
	row, err := repo.FindByVersionPlatform(ctx, query.Version, query.Platform)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.query_failed", nil, "查询客户端版本失败", err)
	}
	return &CurrentCheckResponse{ForceUpdate: row != nil && row.ForceUpdate == enum.CommonYes}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.repository_missing", nil, "客户端版本仓储未配置", ErrRepositoryNotConfigured)
	}
	return s.repository, nil
}

func (s *Service) requirePublisher() (ManifestPublisher, *apperror.Error) {
	if s == nil || s.publisher == nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.publisher_missing", nil, "客户端版本清单发布器未配置", ErrPublisherNotConfigured)
	}
	return s.publisher, nil
}

func (s *Service) withPublishTx(ctx context.Context, fn func(txRepo Repository) error) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if _, appErr := s.requirePublisher(); appErr != nil {
		return appErr
	}
	if err := repo.WithTransaction(ctx, func(txCtx context.Context, txRepo Repository) error {
		return fn(txRepo)
	}); err != nil {
		if strings.Contains(err.Error(), "publish manifest") {
			return apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.manifest_publish_failed", nil, "发布版本更新清单失败", err)
		}
		return apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.save_failed", nil, "保存客户端版本失败", err)
	}
	return nil
}

func (s *Service) publishManifest(ctx context.Context, row Version) error {
	publisher, appErr := s.requirePublisher()
	if appErr != nil {
		return appErr
	}
	body, err := json.Marshal(manifestPayload(row))
	if err != nil {
		return fmt.Errorf("build manifest: %w", err)
	}
	if err := publisher.Publish(ctx, row.Platform, body); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}
	return nil
}

func getVersion(ctx context.Context, repo Repository, id int64) (*Version, *apperror.Error) {
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "clientversion.query_failed", nil, "查询客户端版本失败", err)
	}
	if row == nil {
		return nil, apperror.NotFoundKey("clientversion.not_found", nil, "客户端版本不存在")
	}
	return row, nil
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
	query.Platform = strings.TrimSpace(query.Platform)
	return query
}

func normalizeCreateInput(input CreateInput) (CreateInput, *apperror.Error) {
	input, appErr := normalizeSaveFields(input)
	if appErr != nil {
		return input, appErr
	}
	if input.ForceUpdate == 0 {
		input.ForceUpdate = enum.CommonNo
	}
	if !enum.IsCommonYesNo(input.ForceUpdate) {
		return input, apperror.BadRequestKey("clientversion.force_update.invalid", nil, "无效的强制更新状态")
	}
	return input, nil
}

func normalizeUpdateInput(input UpdateInput) (UpdateInput, *apperror.Error) {
	create, appErr := normalizeSaveFields(CreateInput(input))
	if appErr != nil {
		return input, appErr
	}
	input = UpdateInput(create)
	if input.ForceUpdate == 0 {
		return input, apperror.BadRequestKey("clientversion.force_update.invalid", nil, "无效的强制更新状态")
	}
	if !enum.IsCommonYesNo(input.ForceUpdate) {
		return input, apperror.BadRequestKey("clientversion.force_update.invalid", nil, "无效的强制更新状态")
	}
	return input, nil
}

func normalizeSaveFields(input CreateInput) (CreateInput, *apperror.Error) {
	input.Version = strings.TrimSpace(input.Version)
	input.Notes = strings.TrimSpace(input.Notes)
	input.FileURL = strings.TrimSpace(input.FileURL)
	input.Signature = strings.TrimSpace(input.Signature)
	input.Platform = strings.TrimSpace(input.Platform)
	if input.Version == "" || len([]rune(input.Version)) > 20 {
		return input, apperror.BadRequestKey("clientversion.version.invalid", nil, "版本号不能为空且不能超过20个字符")
	}
	if len([]rune(input.Notes)) > 1000 {
		return input, apperror.BadRequestKey("clientversion.notes.too_long", nil, "版本说明不能超过1000个字符")
	}
	if !enum.IsClientPlatform(input.Platform) {
		return input, apperror.BadRequestKey("clientversion.platform.invalid", nil, "无效的客户端平台")
	}
	if !isHTTPURL(input.FileURL) {
		return input, apperror.BadRequestKey("clientversion.file_url.invalid", nil, "文件地址必须是有效 URL")
	}
	if input.Signature == "" {
		return input, apperror.BadRequestKey("clientversion.signature.required", nil, "签名不能为空")
	}
	if input.FileSize < 0 {
		return input, apperror.BadRequestKey("clientversion.file_size.invalid", nil, "文件大小不能小于0")
	}
	return input, nil
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func manifestPayload(row Version) ManifestPayload {
	at := row.UpdatedAt
	if at.IsZero() {
		at = row.CreatedAt
	}
	if at.IsZero() {
		at = time.Now()
	}
	return ManifestPayload{
		Version: row.Version,
		Notes:   row.Notes,
		PubDate: at.Format(time.RFC3339),
		Platforms: map[string]ManifestPlatform{
			row.Platform: {URL: row.FileURL, Signature: row.Signature},
		},
	}
}

func listItem(row Version) ListItem {
	return ListItem{
		ID:              row.ID,
		Version:         row.Version,
		Notes:           row.Notes,
		FileURL:         row.FileURL,
		Signature:       row.Signature,
		Platform:        row.Platform,
		PlatformName:    enum.ClientPlatformName(row.Platform),
		FileSize:        row.FileSize,
		FileSizeText:    formatFileSize(row.FileSize),
		IsLatest:        row.IsLatest,
		IsLatestName:    yesNoText(row.IsLatest),
		ForceUpdate:     row.ForceUpdate,
		ForceUpdateName: yesNoText(row.ForceUpdate),
		CreatedAt:       formatTime(row.CreatedAt),
		UpdatedAt:       formatTime(row.UpdatedAt),
	}
}

func yesNoText(value int) string {
	if value == enum.CommonYes {
		return "是"
	}
	if value == enum.CommonNo {
		return "否"
	}
	return enum.DefaultNull
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func formatFileSize(size int64) string {
	if size <= 0 {
		return enum.DefaultNull
	}
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.2f %s", math.Round(value*100)/100, units[unit])
}

func totalPage(total int64, pageSize int) int64 {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(total) / float64(pageSize)))
}
