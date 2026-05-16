package uploadconfig

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"sort"
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

func NewService(repository Repository, box *secretbox.Box) *Service {
	service := &Service{repository: repository}
	if box != nil {
		service.secretbox = *box
	}
	return service
}

func (s *Service) DriverInit(ctx context.Context) (*DriverInitResponse, *apperror.Error) {
	return &DriverInitResponse{Dict: DriverInitDict{UploadDriverArr: dict.UploadDriverOptions()}}, nil
}

func (s *Service) RuleInit(ctx context.Context) (*RuleInitResponse, *apperror.Error) {
	return &RuleInitResponse{Dict: RuleInitDict{
		UploadImageExtArr: dict.UploadImageExtOptions(),
		UploadFileExtArr:  dict.UploadFileExtOptions(),
	}}, nil
}

func (s *Service) SettingInit(ctx context.Context) (*SettingInitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	drivers, err := repo.DriverDict(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询上传驱动字典失败", err)
	}
	rules, err := repo.RuleDict(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询上传规则字典失败", err)
	}
	return &SettingInitResponse{Dict: SettingInitDict{
		UploadDriverList: settingDriverOptions(drivers),
		UploadRuleList:   settingRuleOptions(rules),
		CommonStatusArr:  dict.CommonStatusOptions(),
	}}, nil
}

func (s *Service) DriverList(ctx context.Context, query DriverListQuery) (*DriverListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Driver = strings.TrimSpace(query.Driver)
	rows, total, err := repo.ListDrivers(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询上传驱动失败", err)
	}
	list := make([]DriverItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, driverItemFromRow(row))
	}
	return &DriverListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) CreateDriver(ctx context.Context, input DriverCreateInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	input, appErr = normalizeDriverCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsDriverBucket(ctx, input.Driver, input.Bucket, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验上传驱动失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("同一驱动下该桶已存在")
	}
	secretIDEnc, err := s.secretbox.Encrypt(input.SecretID)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密上传驱动 secret_id 失败", err)
	}
	secretKeyEnc, err := s.secretbox.Encrypt(input.SecretKey)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密上传驱动 secret_key 失败", err)
	}
	id, err := repo.CreateDriver(ctx, Driver{
		Driver: input.Driver, SecretIDEnc: secretIDEnc, SecretIDHint: secretbox.Hint(input.SecretID),
		SecretKeyEnc: secretKeyEnc, SecretKeyHint: secretbox.Hint(input.SecretKey),
		Bucket: input.Bucket, Region: input.Region, RoleARN: input.RoleARN, AppID: input.AppID,
		Endpoint: input.Endpoint, BucketDomain: input.BucketDomain, IsDel: enum.CommonNo,
	})
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增上传驱动失败", err)
	}
	return id, nil
}

func (s *Service) UpdateDriver(ctx context.Context, id int64, input DriverUpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的上传驱动ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetDriver(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询上传驱动失败", err)
	}
	if row == nil {
		return apperror.NotFound("上传驱动不存在")
	}
	input, appErr = normalizeDriverUpdateInput(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsDriverBucket(ctx, input.Driver, input.Bucket, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传驱动失败", err)
	}
	if exists {
		return apperror.BadRequest("同一驱动下该桶已存在")
	}
	fields := map[string]any{
		"driver":        input.Driver,
		"bucket":        input.Bucket,
		"region":        input.Region,
		"role_arn":      input.RoleARN,
		"appid":         input.AppID,
		"endpoint":      input.Endpoint,
		"bucket_domain": input.BucketDomain,
	}
	if input.SecretID != "" {
		ciphertext, err := s.secretbox.Encrypt(input.SecretID)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密上传驱动 secret_id 失败", err)
		}
		fields["secret_id_enc"] = ciphertext
		fields["secret_id_hint"] = secretbox.Hint(input.SecretID)
	}
	if input.SecretKey != "" {
		ciphertext, err := s.secretbox.Encrypt(input.SecretKey)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密上传驱动 secret_key 失败", err)
		}
		fields["secret_key_enc"] = ciphertext
		fields["secret_key_hint"] = secretbox.Hint(input.SecretKey)
	}
	if err := repo.UpdateDriver(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新上传驱动失败", err)
	}
	_ = row
	return nil
}

func (s *Service) DeleteDrivers(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的上传驱动")
	}
	referenced, err := repo.DriverReferenced(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传驱动引用失败", err)
	}
	if referenced {
		return apperror.BadRequest("上传驱动已被上传设置引用，无法删除")
	}
	if err := repo.DeleteDrivers(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除上传驱动失败", err)
	}
	return nil
}

func (s *Service) RuleList(ctx context.Context, query RuleListQuery) (*RuleListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Title = strings.TrimSpace(query.Title)
	rows, total, err := repo.ListRules(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询上传规则失败", err)
	}
	list := make([]RuleItem, 0, len(rows))
	for _, row := range rows {
		item, err := ruleItemFromRow(row)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "解析上传规则失败", err)
		}
		list = append(list, item)
	}
	return &RuleListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) CreateRule(ctx context.Context, input RuleMutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeRuleMutation(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsRuleTitle(ctx, row.Title, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验上传规则失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("规则标题已存在")
	}
	id, err := repo.CreateRule(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增上传规则失败", err)
	}
	return id, nil
}

func (s *Service) UpdateRule(ctx context.Context, id int64, input RuleMutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的上传规则ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetRule(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询上传规则失败", err)
	}
	if row == nil {
		return apperror.NotFound("上传规则不存在")
	}
	normalized, appErr := normalizeRuleMutation(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsRuleTitle(ctx, normalized.Title, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传规则失败", err)
	}
	if exists {
		return apperror.BadRequest("规则标题已存在")
	}
	if err := repo.UpdateRule(ctx, id, map[string]any{
		"title":       normalized.Title,
		"max_size_mb": normalized.MaxSizeMB,
		"image_exts":  normalized.ImageExts,
		"file_exts":   normalized.FileExts,
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新上传规则失败", err)
	}
	return nil
}

func (s *Service) DeleteRules(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的上传规则")
	}
	referenced, err := repo.RuleReferenced(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传规则引用失败", err)
	}
	if referenced {
		return apperror.BadRequest("上传规则已被上传设置引用，无法删除")
	}
	if err := repo.DeleteRules(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除上传规则失败", err)
	}
	return nil
}

func (s *Service) SettingList(ctx context.Context, query SettingListQuery) (*SettingListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Remark = strings.TrimSpace(query.Remark)
	rows, total, err := repo.ListSettings(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询上传设置失败", err)
	}
	list := make([]SettingItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, settingItemFromRow(row))
	}
	return &SettingListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) CreateSetting(ctx context.Context, input SettingMutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	input, appErr = normalizeSettingMutationInput(input)
	if appErr != nil {
		return 0, appErr
	}
	if appErr := s.validateSettingReferences(ctx, repo, input, 0); appErr != nil {
		return 0, appErr
	}
	row := Setting{DriverID: input.DriverID, RuleID: input.RuleID, Status: input.Status, Remark: input.Remark, IsDel: enum.CommonNo}
	if input.Status == enum.CommonYes {
		id, err := repo.EnableSettingExclusive(ctx, 0, row, false)
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增上传设置失败", err)
		}
		return id, nil
	}
	id, err := repo.CreateSetting(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增上传设置失败", err)
	}
	return id, nil
}

func (s *Service) UpdateSetting(ctx context.Context, id int64, input SettingMutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的上传设置ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.GetSetting(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询上传设置失败", err)
	}
	if row == nil {
		return apperror.NotFound("上传设置不存在")
	}
	input, appErr = normalizeSettingMutationInput(input)
	if appErr != nil {
		return appErr
	}
	if appErr := s.validateSettingReferences(ctx, repo, input, id); appErr != nil {
		return appErr
	}
	fields := map[string]any{
		"driver_id": input.DriverID,
		"rule_id":   input.RuleID,
		"status":    input.Status,
		"remark":    input.Remark,
	}
	if input.Status == enum.CommonYes {
		updatedID, err := repo.EnableSettingExclusive(ctx, id, Setting{DriverID: input.DriverID, RuleID: input.RuleID, Status: input.Status, Remark: input.Remark}, true)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "更新上传设置失败", err)
		}
		if updatedID == 0 {
			return apperror.NotFound("上传设置不存在")
		}
		return nil
	}
	if err := repo.UpdateSetting(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新上传设置失败", err)
	}
	return nil
}

func (s *Service) ChangeSettingStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的上传设置ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if status == enum.CommonYes {
		updatedID, err := repo.EnableSettingExclusive(ctx, id, Setting{Status: enum.CommonYes}, true)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "更新上传设置状态失败", err)
		}
		if updatedID == 0 {
			return apperror.NotFound("上传设置不存在")
		}
		return nil
	}
	row, err := repo.GetSetting(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询上传设置失败", err)
	}
	if row == nil {
		return apperror.NotFound("上传设置不存在")
	}
	if err := repo.UpdateSetting(ctx, id, map[string]any{"status": enum.CommonNo}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新上传设置状态失败", err)
	}
	return nil
}

func (s *Service) DeleteSettings(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的上传设置")
	}
	enabled, err := repo.SettingEnabledIn(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传设置状态失败", err)
	}
	if enabled {
		return apperror.BadRequest("包含启用的上传设置，无法删除")
	}
	if err := repo.DeleteSettings(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除上传设置失败", err)
	}
	return nil
}

func (s *Service) validateSettingReferences(ctx context.Context, repo Repository, input SettingMutationInput, excludeID int64) *apperror.Error {
	driverOK, err := repo.DriverExists(ctx, input.DriverID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传驱动失败", err)
	}
	if !driverOK {
		return apperror.BadRequest("上传驱动不存在")
	}
	ruleOK, err := repo.RuleExists(ctx, input.RuleID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传规则失败", err)
	}
	if !ruleOK {
		return apperror.BadRequest("上传规则不存在")
	}
	exists, err := repo.ExistsSettingDriverRule(ctx, input.DriverID, input.RuleID, excludeID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验上传设置失败", err)
	}
	if exists {
		return apperror.BadRequest("该驱动与规则组合已存在")
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, repositoryNotConfigured()
	}
	return s.repository, nil
}

func normalizeDriverCreateInput(input DriverCreateInput) (DriverCreateInput, *apperror.Error) {
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	if input.SecretID == "" {
		return input, apperror.BadRequest("secret_id 不能为空")
	}
	if input.SecretKey == "" {
		return input, apperror.BadRequest("secret_key 不能为空")
	}
	update, appErr := normalizeDriverUpdateInput(DriverUpdateInput{
		Driver: input.Driver, Bucket: input.Bucket, Region: input.Region, RoleARN: input.RoleARN,
		AppID: input.AppID, Endpoint: input.Endpoint, BucketDomain: input.BucketDomain,
	})
	if appErr != nil {
		return input, appErr
	}
	input.Driver = update.Driver
	input.Bucket = update.Bucket
	input.Region = update.Region
	input.RoleARN = update.RoleARN
	input.AppID = update.AppID
	input.Endpoint = update.Endpoint
	input.BucketDomain = update.BucketDomain
	return input, nil
}

func normalizeRuleMutation(input RuleMutationInput) (Rule, *apperror.Error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Rule{}, apperror.BadRequest("规则标题不能为空")
	}
	if len([]rune(input.Title)) > 50 {
		return Rule{}, apperror.BadRequest("规则标题不能超过50个字符")
	}
	if input.MaxSizeMB < 1 || input.MaxSizeMB > 10240 {
		return Rule{}, apperror.BadRequest("max_size_mb 必须在 1..10240 之间")
	}
	imageExts, err := enum.NormalizeUploadExts(input.ImageExts, enum.IsUploadImageExt, enum.UploadImageExts)
	if err != nil {
		return Rule{}, apperror.BadRequest("图片扩展名不支持")
	}
	fileExts, err := enum.NormalizeUploadExts(input.FileExts, enum.IsUploadFileExt, enum.UploadFileExts)
	if err != nil {
		return Rule{}, apperror.BadRequest("文件扩展名不支持")
	}
	if len(imageExts) == 0 && len(fileExts) == 0 {
		return Rule{}, apperror.BadRequest("image_exts 和 file_exts 不能同时为空")
	}
	imageJSON, err := json.Marshal(imageExts)
	if err != nil {
		return Rule{}, apperror.BadRequest("图片扩展名格式错误")
	}
	fileJSON, err := json.Marshal(fileExts)
	if err != nil {
		return Rule{}, apperror.BadRequest("文件扩展名格式错误")
	}
	return Rule{
		Title: input.Title, MaxSizeMB: input.MaxSizeMB, ImageExts: string(imageJSON), FileExts: string(fileJSON), IsDel: enum.CommonNo,
	}, nil
}

func normalizeDriverUpdateInput(input DriverUpdateInput) (DriverUpdateInput, *apperror.Error) {
	input.Driver = strings.TrimSpace(input.Driver)
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.Bucket = strings.TrimSpace(input.Bucket)
	input.Region = strings.TrimSpace(input.Region)
	input.RoleARN = strings.TrimSpace(input.RoleARN)
	input.AppID = strings.TrimSpace(input.AppID)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	bucketDomain, appErr := normalizeBucketDomain(input.BucketDomain)
	if appErr != nil {
		return input, appErr
	}
	input.BucketDomain = bucketDomain
	if input.Driver != enum.UploadDriverCOS {
		return input, apperror.BadRequest("当前仅支持腾讯云 COS，请重新配置 COS")
	}
	if input.Bucket == "" {
		return input, apperror.BadRequest("bucket 不能为空")
	}
	if input.Region == "" {
		return input, apperror.BadRequest("region 不能为空")
	}
	if input.AppID == "" {
		return input, apperror.BadRequest("COS appid 不能为空")
	}
	return input, nil
}

func normalizeBucketDomain(domain string) (string, *apperror.Error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", nil
	}
	if strings.Contains(domain, "://") ||
		strings.Contains(domain, "/") ||
		strings.Contains(domain, "?") ||
		strings.Contains(domain, "#") {
		return "", apperror.BadRequest("访问域名请填写裸域名，例如 cos.example.com")
	}
	parsed, err := url.Parse("https://" + domain)
	if err != nil || parsed.Host != domain || parsed.Hostname() == "" {
		return "", apperror.BadRequest("访问域名请填写裸域名，例如 cos.example.com")
	}
	return domain, nil
}

func normalizeSettingMutationInput(input SettingMutationInput) (SettingMutationInput, *apperror.Error) {
	input.Remark = strings.TrimSpace(input.Remark)
	if input.DriverID <= 0 {
		return input, apperror.BadRequest("上传驱动不能为空")
	}
	if input.RuleID <= 0 {
		return input, apperror.BadRequest("上传规则不能为空")
	}
	if !enum.IsCommonStatus(input.Status) {
		return input, apperror.BadRequest("无效的状态")
	}
	if len([]rune(input.Remark)) > 255 {
		return input, apperror.BadRequest("备注不能超过255个字符")
	}
	return input, nil
}

func driverItemFromRow(row Driver) DriverItem {
	return DriverItem{
		ID: row.ID, Driver: row.Driver, DriverShow: enum.UploadDriverLabels[row.Driver],
		SecretIDHint: row.SecretIDHint, SecretKeyHint: row.SecretKeyHint,
		Bucket: row.Bucket, Region: row.Region, RoleARN: optionalString(row.RoleARN), AppID: optionalString(row.AppID),
		Endpoint: optionalString(row.Endpoint), BucketDomain: optionalString(row.BucketDomain),
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func ruleItemFromRow(row Rule) (RuleItem, error) {
	var imageExts []string
	if row.ImageExts != "" {
		if err := json.Unmarshal([]byte(row.ImageExts), &imageExts); err != nil {
			return RuleItem{}, err
		}
	}
	var fileExts []string
	if row.FileExts != "" {
		if err := json.Unmarshal([]byte(row.FileExts), &fileExts); err != nil {
			return RuleItem{}, err
		}
	}
	return RuleItem{
		ID: row.ID, Title: row.Title, MaxSizeMB: row.MaxSizeMB, ImageExts: imageExts, FileExts: fileExts,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}, nil
}

func settingItemFromRow(row SettingListRow) SettingItem {
	driverName := enum.UploadDriverLabels[row.Driver]
	if row.Bucket != "" {
		if driverName == "" {
			driverName = row.Bucket
		} else {
			driverName += " - " + row.Bucket
		}
	}
	return SettingItem{
		ID: row.ID, DriverID: row.DriverID, RuleID: row.RuleID, DriverName: driverName, RuleName: row.RuleTitle,
		Status: row.Status, StatusName: statusLabel(row.Status), Remark: row.Remark,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func settingDriverOptions(rows []Driver) []dict.Option[int] {
	options := make([]dict.Option[int], 0, len(rows))
	for _, row := range rows {
		label := enum.UploadDriverLabels[row.Driver]
		if label == "" {
			label = row.Driver
		}
		if row.Bucket != "" {
			label += " - " + row.Bucket
		}
		options = append(options, dict.Option[int]{Label: label, Value: int(row.ID)})
	}
	return options
}

func settingRuleOptions(rows []Rule) []dict.Option[int] {
	options := make([]dict.Option[int], 0, len(rows))
	for _, row := range rows {
		options = append(options, dict.Option[int]{Label: row.Title, Value: int(row.ID)})
	}
	return options
}

func statusLabel(status int) string {
	for _, item := range dict.CommonStatusOptions() {
		if item.Value == status {
			return item.Label
		}
	}
	return ""
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

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
