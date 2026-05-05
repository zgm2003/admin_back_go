package user

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/permission"

	"golang.org/x/crypto/bcrypt"
)

const defaultButtonCacheTTL = 30 * time.Minute
const timeLayout = "2006-01-02 15:04:05"
const birthdayLayout = "2006-01-02"
const defaultVerifyCodePrefix = "auth:verify_code:"

var (
	userPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)
	userEmailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

type PermissionBuilder interface {
	BuildContextByRole(ctx context.Context, roleID int64, platform string) (permission.Context, *apperror.Error)
}

type ButtonCache interface {
	Set(ctx context.Context, key string, values []string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

type VerifyCodeStore interface {
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type Option func(*Service)

type Service struct {
	repository        Repository
	permissionBuilder PermissionBuilder
	buttonCache       ButtonCache
	buttonCacheTTL    time.Duration
	platforms         []string
	verifyCodeStore   VerifyCodeStore
	verifyCodePrefix  string
}

type addressTreeMutableNode struct {
	ID       int64
	ParentID int64
	Label    string
	Value    int64
	Children []*addressTreeMutableNode
}

func NewService(repository Repository, permissionBuilder PermissionBuilder, buttonCache ButtonCache, buttonCacheTTL time.Duration, opts ...Option) *Service {
	if buttonCacheTTL <= 0 {
		buttonCacheTTL = defaultButtonCacheTTL
	}
	service := &Service{
		repository:        repository,
		permissionBuilder: permissionBuilder,
		buttonCache:       buttonCache,
		buttonCacheTTL:    buttonCacheTTL,
		platforms:         normalizePlatforms(enum.Platforms),
		verifyCodePrefix:  defaultVerifyCodePrefix,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	service.verifyCodePrefix = strings.TrimSpace(service.verifyCodePrefix)
	if service.verifyCodePrefix == "" {
		service.verifyCodePrefix = defaultVerifyCodePrefix
	}
	return service
}

func WithVerifyCodeStore(store VerifyCodeStore, prefix string) Option {
	return func(s *Service) {
		s.verifyCodeStore = store
		s.verifyCodePrefix = prefix
	}
}

func (s *Service) Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error) {
	if input.UserID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户仓储未配置")
	}
	if s.permissionBuilder == nil {
		return nil, apperror.Internal("权限服务未配置")
	}

	currentUser, err := s.repository.FindUser(ctx, input.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return nil, apperror.NotFound("用户不存在")
	}

	profile, err := s.repository.FindProfile(ctx, currentUser.ID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户资料失败", err)
	}

	role, err := s.repository.FindRole(ctx, currentUser.RoleID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}

	roleName := ""
	perm := permission.Context{}
	if role != nil {
		roleName = role.Name
		var appErr *apperror.Error
		perm, appErr = s.permissionBuilder.BuildContextByRole(ctx, role.ID, input.Platform)
		if appErr != nil {
			return nil, appErr
		}
	}

	quickEntry, err := s.repository.QuickEntries(ctx, currentUser.ID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询快捷入口失败", err)
	}

	if role != nil && s.buttonCache != nil {
		_ = s.buttonCache.Set(ctx, permission.ButtonCacheKey(currentUser.ID, input.Platform), perm.ButtonCodes, s.buttonCacheTTL)
	}

	avatar := ""
	if profile != nil {
		avatar = profile.Avatar
	}

	return &InitResponse{
		UserID:      currentUser.ID,
		Username:    currentUser.Username,
		Avatar:      avatar,
		RoleName:    roleName,
		Permissions: perm.Permissions,
		Router:      perm.Router,
		ButtonCodes: perm.ButtonCodes,
		QuickEntry:  quickEntry,
	}, nil
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户仓储未配置")
	}

	roles, err := s.repository.RoleOptions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询角色字典失败", err)
	}
	addresses, err := s.repository.ActiveAddresses(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询地址字典失败", err)
	}

	roleOptions := make([]RoleOption, 0, len(roles))
	for _, role := range roles {
		if role.ID <= 0 {
			continue
		}
		roleOptions = append(roleOptions, RoleOption{Label: role.Name, Value: int(role.ID)})
	}

	return &PageInitResponse{
		Dict: PageInitDict{
			RoleArr:         roleOptions,
			AuthAddressTree: buildAddressTree(addresses),
			SexArr:          dict.SexOptions(),
			PlatformArr:     dict.PlatformOptions(),
		},
	}, nil
}

func (s *Service) Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.BadRequest("无效的用户ID")
	}
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户仓储未配置")
	}

	currentUser, err := s.repository.FindUser(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return nil, apperror.NotFound("用户不存在")
	}

	profile, err := s.repository.FindProfile(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户资料失败", err)
	}
	if profile == nil {
		profile = &Profile{UserID: userID, Sex: enum.SexUnknown}
	}

	role, findRoleErr := s.repository.FindRole(ctx, currentUser.RoleID)
	if findRoleErr != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", findRoleErr)
	}

	addresses, findAddressErr := s.repository.ActiveAddresses(ctx)
	if findAddressErr != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询地址字典失败", findAddressErr)
	}

	return &ProfileResponse{
		Profile: buildProfileDetail(currentUser, profile, role, currentUserID),
		Dict: ProfileDict{
			AuthAddressTree: buildAddressTree(addresses),
			SexArr:          dict.SexOptions(),
			VerifyTypeArr:   dict.UserVerifyTypeOptions(),
		},
	}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户仓储未配置")
	}

	normalized, appErr := normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}

	rows, total, err := s.repository.List(ctx, normalized)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户列表失败", err)
	}
	addresses, err := s.repository.ActiveAddresses(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询地址字典失败", err)
	}
	addressMap := makeAddressMap(addresses)

	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, formatListItem(row, addressMap))
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

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的用户ID")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}

	currentUser, err := s.repository.FindUser(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return apperror.NotFound("用户不存在")
	}

	normalized, appErr := normalizeUpdateInput(input)
	if appErr != nil {
		return appErr
	}
	role, err := s.repository.RoleByID(ctx, normalized.RoleID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}
	if role == nil {
		return apperror.NotFound("角色不存在")
	}

	roleChanged := currentUser.RoleID != normalized.RoleID
	if err := s.repository.WithTx(ctx, func(tx Repository) error {
		if err := tx.UpdateUser(ctx, id, map[string]any{
			"username": normalized.Username,
			"role_id":  normalized.RoleID,
		}); err != nil {
			return err
		}
		return tx.UpdateProfile(ctx, id, map[string]any{
			"avatar":         normalized.Avatar,
			"sex":            normalized.Sex,
			"address_id":     normalized.AddressID,
			"detail_address": normalized.DetailAddress,
			"bio":            normalized.Bio,
		})
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新用户失败", err)
	}

	if roleChanged {
		return s.invalidateUserButtonCache(ctx, id)
	}
	return nil
}

func (s *Service) UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error {
	if input.UserID <= 0 {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}

	currentUser, err := s.repository.FindUser(ctx, input.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return apperror.NotFound("用户不存在")
	}
	normalized, birthday, appErr := normalizeUpdateProfileInput(input)
	if appErr != nil {
		return appErr
	}

	if err := s.repository.WithTx(ctx, func(tx Repository) error {
		if err := tx.UpdateUser(ctx, input.UserID, map[string]any{
			"username": normalized.Username,
		}); err != nil {
			return err
		}
		if _, err := ensureProfileWithRepository(ctx, tx, input.UserID); err != nil {
			return err
		}
		return tx.UpdateProfile(ctx, input.UserID, map[string]any{
			"avatar":         normalized.Avatar,
			"sex":            normalized.Sex,
			"birthday":       birthday,
			"address_id":     normalized.AddressID,
			"detail_address": normalized.DetailAddress,
			"bio":            normalized.Bio,
		})
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新个人资料失败", err)
	}
	return nil
}

func (s *Service) UpdatePassword(ctx context.Context, input UpdatePasswordInput) *apperror.Error {
	if input.UserID <= 0 {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}
	normalized, appErr := normalizeUpdatePasswordInput(input)
	if appErr != nil {
		return appErr
	}

	currentUser, err := s.repository.FindUser(ctx, input.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return apperror.NotFound("用户不存在")
	}

	switch normalized.VerifyType {
	case enum.VerifyTypePassword:
		if currentUser.Password == nil || strings.TrimSpace(*currentUser.Password) == "" {
			return apperror.BadRequest("该账号未设置密码，请使用验证码设置密码")
		}
		if !verifyUserPassword(normalized.OldPassword, *currentUser.Password) {
			return apperror.BadRequest("旧密码错误")
		}
	case enum.VerifyTypeCode:
		if appErr := s.verifyOwnedAccountCode(ctx, currentUser, normalized.Account, normalized.Code, enum.VerifyCodeSceneChangePassword); appErr != nil {
			return appErr
		}
	default:
		return apperror.BadRequest("无效的验证方式")
	}

	hash, err := hashUserPassword(normalized.NewPassword)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "密码加密失败", err)
	}
	if err := s.repository.UpdateUser(ctx, input.UserID, map[string]any{
		"password":   hash,
		"updated_at": time.Now(),
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新密码失败", err)
	}
	return nil
}

func (s *Service) UpdateEmail(ctx context.Context, input UpdateEmailInput) *apperror.Error {
	if input.UserID <= 0 {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}
	normalized, appErr := normalizeUpdateEmailInput(input)
	if appErr != nil {
		return appErr
	}
	currentUser, err := s.repository.FindUser(ctx, input.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return apperror.NotFound("用户不存在")
	}
	exists, err := s.repository.ExistsEmailForOtherUser(ctx, input.UserID, normalized.Email)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验邮箱失败", err)
	}
	if exists {
		return apperror.BadRequest("邮箱已被绑定")
	}
	if appErr := s.verifyCode(ctx, enum.LoginTypeEmail, enum.VerifyCodeSceneBindEmail, normalized.Email, normalized.Code); appErr != nil {
		return appErr
	}
	if err := s.repository.UpdateUser(ctx, input.UserID, map[string]any{
		"email":      normalized.Email,
		"updated_at": time.Now(),
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新邮箱失败", err)
	}
	return nil
}

func (s *Service) UpdatePhone(ctx context.Context, input UpdatePhoneInput) *apperror.Error {
	if input.UserID <= 0 {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}
	normalized, appErr := normalizeUpdatePhoneInput(input)
	if appErr != nil {
		return appErr
	}
	currentUser, err := s.repository.FindUser(ctx, input.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return apperror.NotFound("用户不存在")
	}
	exists, err := s.repository.ExistsPhoneForOtherUser(ctx, input.UserID, normalized.Phone)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验手机号失败", err)
	}
	if exists {
		return apperror.BadRequest("手机号已被绑定")
	}
	if appErr := s.verifyCode(ctx, enum.LoginTypePhone, enum.VerifyCodeSceneBindPhone, normalized.Phone, normalized.Code); appErr != nil {
		return appErr
	}
	if err := s.repository.UpdateUser(ctx, input.UserID, map[string]any{
		"phone":      normalized.Phone,
		"updated_at": time.Now(),
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新手机号失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的用户ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}

	currentUser, err := s.repository.FindUser(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if currentUser == nil {
		return apperror.NotFound("用户不存在")
	}
	if err := s.repository.UpdateStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "修改用户状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的用户")
	}
	if err := s.repository.WithTx(ctx, func(tx Repository) error {
		return tx.SoftDelete(ctx, ids)
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除用户失败", err)
	}
	return nil
}

func (s *Service) BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) *apperror.Error {
	if s == nil || s.repository == nil {
		return apperror.Internal("用户仓储未配置")
	}
	normalized, appErr := normalizeBatchProfileUpdate(input)
	if appErr != nil {
		return appErr
	}
	if err := s.repository.BatchUpdateProfile(ctx, normalized); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "批量修改用户资料失败", err)
	}
	return nil
}

func ensureProfileWithRepository(ctx context.Context, repository Repository, userID int64) (*Profile, error) {
	profile, err := repository.FindProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile != nil {
		return profile, nil
	}
	profile = &Profile{
		UserID:    userID,
		Sex:       enum.SexUnknown,
		IsDel:     enum.CommonNo,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repository.EnsureProfile(ctx, *profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *Service) invalidateUserButtonCache(ctx context.Context, userID int64) *apperror.Error {
	if s.buttonCache == nil {
		return nil
	}
	for _, platform := range s.platforms {
		if err := s.buttonCache.Delete(ctx, permission.ButtonCacheKey(userID, platform)); err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "清理用户权限缓存失败", err)
		}
	}
	return nil
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		return query, apperror.BadRequest("当前页无效")
	}
	if query.PageSize < enum.PageSizeMin || query.PageSize > enum.PageSizeMax {
		return query, apperror.BadRequest("每页数量无效")
	}
	if query.Sex != nil && !enum.IsSex(*query.Sex) {
		return query, apperror.BadRequest("无效的性别")
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Username = strings.TrimSpace(query.Username)
	query.Email = strings.TrimSpace(query.Email)
	query.DetailAddress = strings.TrimSpace(query.DetailAddress)
	query.AddressIDs = normalizeIDs(query.AddressIDs)
	query.DateRange = normalizeDateRange(query.DateRange)
	return query, nil
}

func normalizeUpdateInput(input UpdateInput) (UpdateInput, *apperror.Error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Avatar = strings.TrimSpace(input.Avatar)
	input.DetailAddress = strings.TrimSpace(input.DetailAddress)
	input.Bio = strings.TrimSpace(input.Bio)
	if input.Username == "" {
		return input, apperror.BadRequest("用户名不能为空")
	}
	if len([]rune(input.Username)) > 64 {
		return input, apperror.BadRequest("用户名不能超过64个字符")
	}
	if input.RoleID <= 0 {
		return input, apperror.BadRequest("角色不能为空")
	}
	if !enum.IsSex(input.Sex) {
		return input, apperror.BadRequest("无效的性别")
	}
	if input.AddressID < 0 {
		return input, apperror.BadRequest("无效的地址")
	}
	return input, nil
}

func normalizeUpdateProfileInput(input UpdateProfileInput) (UpdateProfileInput, *time.Time, *apperror.Error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Avatar = strings.TrimSpace(input.Avatar)
	input.DetailAddress = strings.TrimSpace(input.DetailAddress)
	input.Bio = strings.TrimSpace(input.Bio)
	if input.Username == "" {
		return input, nil, apperror.BadRequest("用户名不能为空")
	}
	if len([]rune(input.Username)) > 64 {
		return input, nil, apperror.BadRequest("用户名不能超过64个字符")
	}
	if !enum.IsSex(input.Sex) {
		return input, nil, apperror.BadRequest("无效的性别")
	}
	if input.AddressID < 0 {
		return input, nil, apperror.BadRequest("无效的地址")
	}

	birthday, appErr := parseBirthday(input.Birthday)
	if appErr != nil {
		return input, nil, appErr
	}
	return input, birthday, nil
}

func parseBirthday(value *string) (*time.Time, *apperror.Error) {
	if value == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation(birthdayLayout, raw, time.Local)
	if err != nil {
		return nil, apperror.BadRequest("生日格式错误")
	}
	return &parsed, nil
}

func normalizeUpdatePasswordInput(input UpdatePasswordInput) (UpdatePasswordInput, *apperror.Error) {
	input.VerifyType = strings.TrimSpace(input.VerifyType)
	input.OldPassword = strings.TrimSpace(input.OldPassword)
	input.Account = strings.TrimSpace(input.Account)
	input.Code = strings.TrimSpace(input.Code)
	input.NewPassword = strings.TrimSpace(input.NewPassword)
	input.ConfirmPassword = strings.TrimSpace(input.ConfirmPassword)
	if !enum.IsUserVerifyType(input.VerifyType) {
		return input, apperror.BadRequest("无效的验证方式")
	}
	if input.NewPassword == "" || input.ConfirmPassword == "" {
		return input, apperror.BadRequest("请输入新密码")
	}
	if input.NewPassword != input.ConfirmPassword {
		return input, apperror.BadRequest("两次输入的密码不一致")
	}
	passwordLength := len([]rune(input.NewPassword))
	if passwordLength < 6 || passwordLength > 128 {
		return input, apperror.BadRequest("密码长度必须为6-128位")
	}
	switch input.VerifyType {
	case enum.VerifyTypePassword:
		if input.OldPassword == "" {
			return input, apperror.BadRequest("请输入旧密码")
		}
	case enum.VerifyTypeCode:
		if input.Account == "" {
			return input, apperror.BadRequest("验证码接收账号不能为空")
		}
		if input.Code == "" {
			return input, apperror.BadRequest("请输入验证码")
		}
	}
	return input, nil
}

func normalizeUpdateEmailInput(input UpdateEmailInput) (UpdateEmailInput, *apperror.Error) {
	input.Email = strings.TrimSpace(input.Email)
	input.Code = strings.TrimSpace(input.Code)
	if !userEmailPattern.MatchString(input.Email) {
		return input, apperror.BadRequest("邮箱格式不正确")
	}
	if input.Code == "" {
		return input, apperror.BadRequest("请输入验证码")
	}
	return input, nil
}

func normalizeUpdatePhoneInput(input UpdatePhoneInput) (UpdatePhoneInput, *apperror.Error) {
	input.Phone = strings.TrimSpace(input.Phone)
	input.Code = strings.TrimSpace(input.Code)
	if !userPhonePattern.MatchString(input.Phone) {
		return input, apperror.BadRequest("手机号格式不正确")
	}
	if input.Code == "" {
		return input, apperror.BadRequest("请输入验证码")
	}
	return input, nil
}

func (s *Service) verifyOwnedAccountCode(ctx context.Context, currentUser *User, account string, code string, scene string) *apperror.Error {
	accountType := accountTypeOf(account)
	if accountType == "" {
		return apperror.BadRequest("请输入正确的邮箱或手机号")
	}
	switch accountType {
	case enum.LoginTypeEmail:
		if strings.TrimSpace(currentUser.Email) == "" {
			return apperror.BadRequest("请先绑定邮箱或手机号")
		}
		if strings.TrimSpace(currentUser.Email) != account {
			return apperror.BadRequest("验证码账号不属于当前用户")
		}
	case enum.LoginTypePhone:
		if strings.TrimSpace(currentUser.Phone) == "" {
			return apperror.BadRequest("请先绑定邮箱或手机号")
		}
		if strings.TrimSpace(currentUser.Phone) != account {
			return apperror.BadRequest("验证码账号不属于当前用户")
		}
	}
	return s.verifyCode(ctx, accountType, scene, account, code)
}

func (s *Service) verifyCode(ctx context.Context, accountType string, scene string, account string, code string) *apperror.Error {
	if s == nil || s.verifyCodeStore == nil {
		return apperror.Internal("验证码缓存未配置")
	}
	key := auth.VerifyCodeCacheKey(s.verifyCodePrefix, accountType, scene, account)
	cached, err := s.verifyCodeStore.Get(ctx, key)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "验证码缓存读取失败", err)
	}
	if cached == "" || cached != strings.TrimSpace(code) {
		return apperror.BadRequest("验证码错误或已失效")
	}
	if err := s.verifyCodeStore.Delete(ctx, key); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "验证码消费失败", err)
	}
	return nil
}

func accountTypeOf(value string) string {
	value = strings.TrimSpace(value)
	if userEmailPattern.MatchString(value) {
		return enum.LoginTypeEmail
	}
	if userPhonePattern.MatchString(value) {
		return enum.LoginTypePhone
	}
	return ""
}

func hashUserPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return strings.Replace(string(hash), "$2a$", "$2y$", 1), nil
}

func verifyUserPassword(password string, hash string) bool {
	hash = strings.TrimSpace(hash)
	if strings.HasPrefix(hash, "$2y$") {
		hash = "$2a$" + strings.TrimPrefix(hash, "$2y$")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func buildProfileDetail(currentUser *User, profile *Profile, role *Role, currentUserID int64) ProfileDetail {
	roleName := ""
	if role != nil {
		roleName = role.Name
	}
	isSelf := enum.CommonNo
	if currentUserID > 0 && currentUser.ID == currentUserID {
		isSelf = enum.CommonYes
	}
	detail := ProfileDetail{
		UserID:      currentUser.ID,
		Username:    currentUser.Username,
		Email:       currentUser.Email,
		Phone:       currentUser.Phone,
		RoleID:      currentUser.RoleID,
		RoleName:    roleName,
		IsSelf:      isSelf,
		HasPassword: currentUser.Password != nil && strings.TrimSpace(*currentUser.Password) != "",
	}
	if profile != nil {
		detail.Avatar = profile.Avatar
		detail.AddressID = profile.AddressID
		detail.DetailAddress = profile.DetailAddress
		detail.Sex = profile.Sex
		detail.Bio = profile.Bio
		if profile.Birthday != nil && !profile.Birthday.IsZero() {
			detail.Birthday = profile.Birthday.Format(birthdayLayout)
		}
	}
	return detail
}

func normalizeBatchProfileUpdate(input BatchProfileUpdate) (BatchProfileUpdate, *apperror.Error) {
	input.IDs = normalizeIDs(input.IDs)
	input.DetailAddress = strings.TrimSpace(input.DetailAddress)
	if len(input.IDs) == 0 {
		return input, apperror.BadRequest("请选择要修改的用户")
	}

	switch input.Field {
	case BatchProfileFieldSex:
		if !enum.IsSex(input.Sex) {
			return input, apperror.BadRequest("无效的性别")
		}
	case BatchProfileFieldAddressID:
		if input.AddressID <= 0 {
			return input, apperror.BadRequest("地址不能为空")
		}
	case BatchProfileFieldDetailAddress:
		if input.DetailAddress == "" {
			return input, apperror.BadRequest("详细地址不能为空")
		}
	default:
		return input, apperror.BadRequest("无效的批量修改字段")
	}
	return input, nil
}

func formatListItem(row ListRow, addressMap map[int64]Address) ListItem {
	sex := enum.SexUnknown
	if row.Sex != nil {
		sex = *row.Sex
	}
	addressID := int64(0)
	if row.AddressID != nil {
		addressID = *row.AddressID
	}
	detailAddress := ""
	if row.DetailAddress != nil {
		detailAddress = *row.DetailAddress
	}
	bio := ""
	if row.Bio != nil {
		bio = *row.Bio
	}

	return ListItem{
		ID:            row.ID,
		Username:      row.Username,
		Email:         row.Email,
		Phone:         row.Phone,
		Avatar:        row.Avatar,
		Sex:           sex,
		SexShow:       sexLabel(sex),
		RoleID:        row.RoleID,
		RoleName:      row.RoleName,
		Bio:           bio,
		AddressShow:   buildAddressShow(addressID, detailAddress, addressMap),
		AddressID:     addressID,
		DetailAddress: detailAddress,
		Status:        row.Status,
		CreatedAt:     formatTime(row.CreatedAt),
	}
}

func sexLabel(value int) string {
	for _, option := range dict.SexOptions() {
		if option.Value == value {
			return option.Label
		}
	}
	return "未知"
}

func buildAddressTree(rows []Address) []AddressTreeNode {
	nodes := make(map[int64]*addressTreeMutableNode, len(rows))
	order := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ID <= 0 {
			continue
		}
		node := &addressTreeMutableNode{
			ID:       row.ID,
			ParentID: row.ParentID,
			Label:    row.Name,
			Value:    row.ID,
		}
		nodes[row.ID] = node
		order = append(order, row.ID)
	}

	roots := make([]*addressTreeMutableNode, 0)
	for _, id := range order {
		node := nodes[id]
		if node == nil {
			continue
		}
		parent := nodes[node.ParentID]
		if parent == nil {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	result := make([]AddressTreeNode, 0, len(roots))
	for _, root := range roots {
		result = append(result, freezeAddressNode(root))
	}
	return result
}

func freezeAddressNode(node *addressTreeMutableNode) AddressTreeNode {
	if node == nil {
		return AddressTreeNode{}
	}
	children := make([]AddressTreeNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, freezeAddressNode(child))
	}
	return AddressTreeNode{
		ID:       node.ID,
		ParentID: node.ParentID,
		Label:    node.Label,
		Value:    node.Value,
		Children: children,
	}
}

func makeAddressMap(rows []Address) map[int64]Address {
	result := make(map[int64]Address, len(rows))
	for _, row := range rows {
		if row.ID <= 0 {
			continue
		}
		result[row.ID] = row
	}
	return result
}

func buildAddressShow(addressID int64, detail string, addressMap map[int64]Address) string {
	parts := make([]string, 0, 4)
	for _, name := range buildAddressPath(addressID, addressMap) {
		if name != "" {
			parts = append(parts, name)
		}
	}
	if detail != "" {
		parts = append(parts, detail)
	}
	return strings.Join(parts, "-")
}

func buildAddressPath(addressID int64, addressMap map[int64]Address) []string {
	if addressID <= 0 {
		return nil
	}
	visited := map[int64]struct{}{}
	path := make([]string, 0, 4)
	for currentID := addressID; currentID > 0; {
		if _, ok := visited[currentID]; ok {
			break
		}
		visited[currentID] = struct{}{}
		row, ok := addressMap[currentID]
		if !ok {
			break
		}
		path = append(path, row.Name)
		currentID = row.ParentID
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
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

func normalizePlatforms(platforms []string) []string {
	seen := make(map[string]struct{}, len(platforms))
	result := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)
		if platform == "" {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		result = append(result, platform)
	}
	sort.Strings(result)
	return result
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
