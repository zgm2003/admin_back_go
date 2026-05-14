package authplatform

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/session"
)

const (
	minCodeLen     = 2
	maxCodeLen     = 49
	maxNameLen     = 100
	minAccessTTL   = 60
	maxAccessTTL   = 2592000
	minRefreshTTL  = 60
	maxRefreshTTL  = 31536000
	minMaxSessions = 0
	maxMaxSessions = 100
	timeLayout     = "2006-01-02 15:04:05"
)

var platformCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,48}$`)
var ErrRepositoryNotConfigured = errors.New("auth platform repository is not configured")

type Platform struct {
	ID            int64     `gorm:"column:id"`
	Code          string    `gorm:"column:code"`
	Name          string    `gorm:"column:name"`
	LoginTypes    string    `gorm:"column:login_types"`
	CaptchaType   string    `gorm:"column:captcha_type"`
	BindPlatform  int       `gorm:"column:bind_platform"`
	BindDevice    int       `gorm:"column:bind_device"`
	BindIP        int       `gorm:"column:bind_ip"`
	SingleSession int       `gorm:"column:single_session"`
	MaxSessions   int       `gorm:"column:max_sessions"`
	AllowRegister int       `gorm:"column:allow_register"`
	AccessTTL     int       `gorm:"column:access_ttl"`
	RefreshTTL    int       `gorm:"column:refresh_ttl"`
	Status        int       `gorm:"column:status"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Platform) TableName() string {
	return "auth_platforms"
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Policy(ctx context.Context, platform string) (*session.AuthPolicy, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}

	row, err := s.activePlatform(ctx, platform)
	if err != nil || row == nil {
		return nil, err
	}

	return &session.AuthPolicy{
		BindPlatform:             row.BindPlatform == enum.CommonYes,
		BindDevice:               row.BindDevice == enum.CommonYes,
		BindIP:                   row.BindIP == enum.CommonYes,
		SingleSessionPerPlatform: row.SingleSession == enum.CommonYes,
		MaxSessions:              row.MaxSessions,
		AllowRegister:            row.AllowRegister == enum.CommonYes,
		AccessTTL:                time.Duration(row.AccessTTL) * time.Second,
		RefreshTTL:               time.Duration(row.RefreshTTL) * time.Second,
	}, nil
}

func (s *Service) LoginTypes(ctx context.Context, platform string) ([]string, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}

	row, err := s.activePlatform(ctx, platform)
	if err != nil || row == nil {
		return nil, err
	}
	return normalizeLoginTypes(row.LoginTypes), nil
}

func (s *Service) CaptchaType(ctx context.Context, platform string) (string, error) {
	if s == nil || s.repository == nil {
		return "", ErrRepositoryNotConfigured
	}

	row, err := s.activePlatform(ctx, platform)
	if err != nil || row == nil {
		return "", err
	}
	captchaType := strings.TrimSpace(row.CaptchaType)
	if !enum.IsCaptchaType(captchaType) {
		return "", errors.New("invalid auth platform captcha_type")
	}
	return captchaType, nil
}

func (s *Service) AllowRegister(ctx context.Context, platform string) (bool, error) {
	if s == nil || s.repository == nil {
		return false, ErrRepositoryNotConfigured
	}

	row, err := s.activePlatform(ctx, platform)
	if err != nil || row == nil {
		return false, err
	}
	return row.AllowRegister == enum.CommonYes, nil
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		CommonStatusArr:            dict.CommonStatusOptions(),
		AuthPlatformLoginTypeArr:   dict.AuthPlatformLoginTypeOptions(),
		AuthPlatformCaptchaTypeArr: dict.AuthPlatformCaptchaTypeOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.managementRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := validateListQuery(query); appErr != nil {
		return nil, appErr
	}
	query.Name = strings.TrimSpace(query.Name)

	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.query_failed", nil, "查询认证平台失败", err)
	}

	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItemFromPlatform(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	repo, appErr := s.managementRepository()
	if appErr != nil {
		return 0, appErr
	}
	input, appErr = normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByCode(ctx, input.Code, 0)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.code_check_failed", nil, "校验平台标识失败", err)
	}
	if exists {
		return 0, apperror.BadRequestKey("authplatform.code.duplicate", map[string]any{"code": input.Code}, "平台标识 ["+input.Code+"] 已存在")
	}

	row, appErr := platformFromCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.create_failed", nil, "新增认证平台失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("authplatform.id.invalid", nil, "无效的平台ID")
	}
	repo, appErr := s.managementRepository()
	if appErr != nil {
		return appErr
	}
	existing, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.query_failed", nil, "查询认证平台失败", err)
	}
	if existing == nil {
		return apperror.NotFoundKey("authplatform.not_found", nil, "认证平台不存在")
	}
	input, appErr = normalizeUpdateInput(input)
	if appErr != nil {
		return appErr
	}
	fields, appErr := updateFieldsFromInput(input)
	if appErr != nil {
		return appErr
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.update_failed", nil, "更新认证平台失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.managementRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequestKey("authplatform.delete.empty", nil, "请选择要删除的平台")
	}
	rows, err := repo.PlatformsByIDs(ctx, ids)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.query_failed", nil, "查询认证平台失败", err)
	}
	if len(rows) != len(ids) {
		return apperror.BadRequestKey("authplatform.delete.contains_missing", nil, "包含不存在的平台")
	}
	for _, id := range ids {
		if rows[id].Code == enum.PlatformAdmin {
			return apperror.BadRequestKey("authplatform.delete.admin_forbidden", nil, "核心平台 [admin] 不允许删除")
		}
	}
	if err := repo.Delete(ctx, ids); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.delete_failed", nil, "删除认证平台失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("authplatform.id.invalid", nil, "无效的平台ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequestKey("authplatform.status.invalid", nil, "无效的状态")
	}
	repo, appErr := s.managementRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.query_failed", nil, "查询认证平台失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("authplatform.not_found", nil, "认证平台不存在")
	}
	if row.Code == enum.PlatformAdmin && status == enum.CommonNo {
		return apperror.BadRequestKey("authplatform.status.disable_forbidden", nil, "核心平台 [admin] 不允许禁用")
	}
	if err := repo.Update(ctx, id, map[string]any{"status": status}); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.status_update_failed", nil, "更新认证平台状态失败", err)
	}
	return nil
}

func (s *Service) activePlatform(ctx context.Context, platform string) (*Platform, error) {
	code := strings.TrimSpace(platform)
	if code == "" {
		return nil, nil
	}
	return s.repository.FindActiveByCode(ctx, code)
}

func (s *Service) managementRepository() (ManagementRepository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, managementRepositoryNotConfigured()
	}
	repo, ok := s.repository.(ManagementRepository)
	if !ok {
		return nil, managementRepositoryNotConfigured()
	}
	return repo, nil
}

func validateListQuery(query ListQuery) *apperror.Error {
	if query.CurrentPage <= 0 {
		return apperror.BadRequestKey("authplatform.current_page.invalid", nil, "当前页无效")
	}
	if query.PageSize < enum.PageSizeMin || query.PageSize > enum.PageSizeMax {
		return apperror.BadRequestKey("authplatform.page_size.invalid", nil, "每页数量无效")
	}
	if query.Status != nil && !enum.IsCommonStatus(*query.Status) {
		return apperror.BadRequestKey("authplatform.status.invalid", nil, "无效的状态")
	}
	return nil
}

func normalizeCreateInput(input CreateInput) (CreateInput, *apperror.Error) {
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.CaptchaType = strings.TrimSpace(input.CaptchaType)
	if len(input.Code) < minCodeLen || len(input.Code) > maxCodeLen || !platformCodePattern.MatchString(input.Code) {
		return input, apperror.BadRequestKey("authplatform.code.invalid", nil, "平台标识格式错误")
	}
	update, appErr := normalizeUpdateInput(UpdateInput{
		Name: input.Name, LoginTypes: input.LoginTypes, CaptchaType: input.CaptchaType,
		AccessTTL: input.AccessTTL, RefreshTTL: input.RefreshTTL, BindPlatform: input.BindPlatform, BindDevice: input.BindDevice,
		BindIP: input.BindIP, SingleSession: input.SingleSession, MaxSessions: input.MaxSessions, AllowRegister: input.AllowRegister,
	})
	if appErr != nil {
		return input, appErr
	}
	input.Name = update.Name
	input.LoginTypes = update.LoginTypes
	input.CaptchaType = update.CaptchaType
	input.AccessTTL = update.AccessTTL
	input.RefreshTTL = update.RefreshTTL
	input.BindPlatform = update.BindPlatform
	input.BindDevice = update.BindDevice
	input.BindIP = update.BindIP
	input.SingleSession = update.SingleSession
	input.MaxSessions = update.MaxSessions
	input.AllowRegister = update.AllowRegister
	return input, nil
}

func normalizeUpdateInput(input UpdateInput) (UpdateInput, *apperror.Error) {
	input.Name = strings.TrimSpace(input.Name)
	input.CaptchaType = strings.TrimSpace(input.CaptchaType)
	if input.Name == "" || len([]rune(input.Name)) > maxNameLen {
		return input, apperror.BadRequestKey("authplatform.name.invalid", nil, "平台名称不能为空且不能超过100个字符")
	}
	loginTypes, appErr := normalizeLoginTypesForWrite(input.LoginTypes)
	if appErr != nil {
		return input, appErr
	}
	input.LoginTypes = loginTypes
	if !enum.IsCaptchaType(input.CaptchaType) {
		return input, apperror.BadRequestKey("authplatform.captcha_type.invalid", nil, "无效的验证码类型")
	}
	if input.AccessTTL < minAccessTTL || input.AccessTTL > maxAccessTTL {
		return input, apperror.BadRequestKey("authplatform.access_ttl.invalid", nil, "access_token有效期无效")
	}
	if input.RefreshTTL < minRefreshTTL || input.RefreshTTL > maxRefreshTTL {
		return input, apperror.BadRequestKey("authplatform.refresh_ttl.invalid", nil, "refresh_token有效期无效")
	}
	if !enum.IsCommonYesNo(input.BindPlatform) || !enum.IsCommonYesNo(input.BindDevice) || !enum.IsCommonYesNo(input.BindIP) || !enum.IsCommonYesNo(input.SingleSession) || !enum.IsCommonYesNo(input.AllowRegister) {
		return input, apperror.BadRequestKey("authplatform.policy.invalid", nil, "安全策略参数无效")
	}
	if input.MaxSessions < minMaxSessions || input.MaxSessions > maxMaxSessions {
		return input, apperror.BadRequestKey("authplatform.max_sessions.invalid", nil, "最大会话数无效")
	}
	return input, nil
}

func normalizeLoginTypesForWrite(values []string) ([]string, *apperror.Error) {
	if len(values) == 0 {
		return nil, apperror.BadRequestKey("authplatform.login_types.empty", nil, "登录方式不能为空")
	}
	allowed := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !enum.IsLoginType(value) {
			return nil, apperror.BadRequestKey("authplatform.login_types.invalid", nil, "无效的登录方式")
		}
		allowed[value] = struct{}{}
	}
	result := make([]string, 0, len(enum.LoginTypes))
	for _, value := range enum.LoginTypes {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return nil, apperror.BadRequestKey("authplatform.login_types.empty", nil, "登录方式不能为空")
	}
	return result, nil
}

func platformFromCreateInput(input CreateInput) (Platform, *apperror.Error) {
	loginTypes, appErr := marshalLoginTypes(input.LoginTypes)
	if appErr != nil {
		return Platform{}, appErr
	}
	return Platform{
		Code: input.Code, Name: input.Name, LoginTypes: loginTypes, CaptchaType: input.CaptchaType,
		BindPlatform: input.BindPlatform, BindDevice: input.BindDevice, BindIP: input.BindIP,
		SingleSession: input.SingleSession, MaxSessions: input.MaxSessions, AllowRegister: input.AllowRegister,
		AccessTTL: input.AccessTTL, RefreshTTL: input.RefreshTTL, Status: enum.CommonYes, IsDel: enum.CommonNo,
	}, nil
}

func updateFieldsFromInput(input UpdateInput) (map[string]any, *apperror.Error) {
	loginTypes, appErr := marshalLoginTypes(input.LoginTypes)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{
		"name": input.Name, "login_types": loginTypes, "captcha_type": input.CaptchaType,
		"access_ttl": input.AccessTTL, "refresh_ttl": input.RefreshTTL,
		"bind_platform": input.BindPlatform, "bind_device": input.BindDevice, "bind_ip": input.BindIP,
		"single_session": input.SingleSession, "max_sessions": input.MaxSessions, "allow_register": input.AllowRegister,
	}, nil
}

func marshalLoginTypes(values []string) (string, *apperror.Error) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", apperror.WrapKey(apperror.CodeInternal, 500, "authplatform.encode_login_types_failed", nil, "编码登录方式失败", err)
	}
	return string(encoded), nil
}

func listItemFromPlatform(row Platform) ListItem {
	return ListItem{
		ID: row.ID, Code: row.Code, Name: row.Name, LoginTypes: normalizeLoginTypes(row.LoginTypes), CaptchaType: row.CaptchaType,
		AccessTTL: row.AccessTTL, RefreshTTL: row.RefreshTTL, BindPlatform: row.BindPlatform, BindDevice: row.BindDevice,
		BindIP: row.BindIP, SingleSession: row.SingleSession, MaxSessions: row.MaxSessions, AllowRegister: row.AllowRegister,
		Status: row.Status, StatusName: statusLabel(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func normalizeLoginTypes(raw string) []string {
	var decoded []string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return []string{}
	}
	allowed := make(map[string]struct{}, len(decoded))
	for _, value := range decoded {
		value = strings.TrimSpace(value)
		if value != "" {
			allowed[value] = struct{}{}
		}
	}

	result := make([]string, 0, len(enum.LoginTypes))
	for _, value := range enum.LoginTypes {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
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
