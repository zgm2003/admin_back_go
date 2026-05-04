package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/platform/taskqueue"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

const (
	LoginTypeEmail    = enum.LoginTypeEmail
	LoginTypePhone    = enum.LoginTypePhone
	LoginTypePassword = enum.LoginTypePassword

	VerifyCodeSceneLogin          = enum.VerifyCodeSceneLogin
	VerifyCodeSceneForget         = enum.VerifyCodeSceneForget
	VerifyCodeSceneBindPhone      = enum.VerifyCodeSceneBindPhone
	VerifyCodeSceneBindEmail      = enum.VerifyCodeSceneBindEmail
	VerifyCodeSceneChangePassword = enum.VerifyCodeSceneChangePassword
)

const (
	defaultVerifyCodeTTL = 5 * time.Minute
	defaultDevCode       = "123456"
	profileSexUnknown    = 0
)

var (
	phonePattern           = regexp.MustCompile(`^1[3-9]\d{9}$`)
	emailPattern           = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	errDefaultRoleNotFound = errors.New("default role not found")
)

type PlatformConfigProvider interface {
	LoginTypes(ctx context.Context, platform string) ([]string, error)
	CaptchaType(ctx context.Context, platform string) (string, error)
	AllowRegister(ctx context.Context, platform string) (bool, error)
}

type SessionManager interface {
	Create(ctx context.Context, input session.CreateInput) (*session.TokenResult, *apperror.Error)
	Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error)
	Logout(ctx context.Context, accessToken string) *apperror.Error
}

type CaptchaVerifier interface {
	Verify(ctx context.Context, input captcha.VerifyInput) *apperror.Error
}

type VerifyCodeOptions struct {
	TTL           time.Duration
	RedisPrefix   string
	DevMode       bool
	DevCode       string
	CodeGenerator func() (string, error)
}

type Option func(*Service)

type Service struct {
	repository        Repository
	platformConfig    PlatformConfigProvider
	sessionManager    SessionManager
	captchaVerifier   CaptchaVerifier
	codeStore         CodeStore
	loginLogEnqueuer  taskqueue.Enqueuer
	logger            *slog.Logger
	verifyCodeOptions VerifyCodeOptions
}

func NewService(repository Repository, platformConfig PlatformConfigProvider, sessionManager SessionManager, captchaVerifier CaptchaVerifier, opts ...Option) *Service {
	service := &Service{
		repository:      repository,
		platformConfig:  platformConfig,
		sessionManager:  sessionManager,
		captchaVerifier: captchaVerifier,
		logger:          slog.Default(),
		verifyCodeOptions: VerifyCodeOptions{
			TTL:         defaultVerifyCodeTTL,
			RedisPrefix: defaultVerifyCodeRedisPrefix,
			DevCode:     defaultDevCode,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	service.verifyCodeOptions = normalizeVerifyCodeOptions(service.verifyCodeOptions)
	return service
}

func WithCodeStore(store CodeStore) Option {
	return func(s *Service) {
		s.codeStore = store
	}
}

func WithLoginLogEnqueuer(enqueuer taskqueue.Enqueuer) Option {
	return func(s *Service) {
		s.loginLogEnqueuer = enqueuer
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func WithVerifyCodeOptions(options VerifyCodeOptions) Option {
	return func(s *Service) {
		s.verifyCodeOptions = options
	}
}

func (s *Service) LoginConfig(ctx context.Context, platform string) (*LoginConfigResponse, *apperror.Error) {
	loginTypes, appErr := s.allowedLoginTypes(ctx, platform)
	if appErr != nil {
		return nil, appErr
	}
	orderedLoginTypes := loginConfigOrder(loginTypes)
	options := make([]LoginTypeOption, 0, len(orderedLoginTypes))
	for _, loginType := range orderedLoginTypes {
		options = append(options, LoginTypeOption{Label: loginTypeLabel(loginType), Value: loginType})
	}
	captchaType, appErr := s.captchaType(ctx, platform)
	if appErr != nil {
		return nil, appErr
	}
	return &LoginConfigResponse{
		LoginTypeArr:   options,
		CaptchaEnabled: true,
		CaptchaType:    captchaType,
	}, nil
}

func (s *Service) SendCode(ctx context.Context, input SendCodeInput) (string, *apperror.Error) {
	if s == nil || s.codeStore == nil {
		return "", apperror.Internal("验证码缓存未配置")
	}
	input = normalizeSendCodeInput(input)
	if input.Account == "" {
		return "", apperror.BadRequest("账号不能为空")
	}
	if !enum.IsVerifyCodeScene(input.Scene) {
		return "", apperror.BadRequest("无效的验证码场景")
	}

	accountType := accountTypeOf(input.Account)
	if accountType == "" {
		return "", apperror.BadRequest("请输入正确的邮箱或手机号")
	}

	if !s.verifyCodeOptions.DevMode {
		if accountType == LoginTypeEmail {
			return "", apperror.Internal("邮件验证码服务未配置")
		}
		return "", apperror.Internal("短信验证码服务未配置")
	}

	code, err := s.generateVerifyCode()
	if err != nil {
		return "", apperror.Internal("验证码生成失败")
	}
	if err := s.codeStore.Set(ctx, s.verifyCodeCacheKey(accountType, input.Scene, input.Account), code, s.verifyCodeOptions.TTL); err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "验证码缓存写入失败", err)
	}
	return "验证码发送成功(测试:" + code + ")", nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*LoginResponse, *apperror.Error) {
	if s == nil || s.repository == nil || s.sessionManager == nil {
		return nil, apperror.Unauthorized("登录服务未配置")
	}
	input = normalizeLoginInput(input)
	if input.Platform == "" {
		return nil, apperror.BadRequest("缺少平台标识")
	}
	if input.LoginAccount == "" {
		return nil, apperror.BadRequest("登录账号不能为空")
	}
	if appErr := s.assertLoginTypeAllowed(ctx, input.Platform, input.LoginType); appErr != nil {
		return nil, appErr
	}

	var (
		user      *UserCredential
		isNewUser bool
		appErr    *apperror.Error
	)
	switch input.LoginType {
	case LoginTypePassword:
		user, appErr = s.loginByPassword(ctx, input)
	case LoginTypeEmail, LoginTypePhone:
		user, isNewUser, appErr = s.loginByCode(ctx, input)
	default:
		appErr = apperror.BadRequest("当前平台不支持该登录方式")
	}
	if appErr != nil {
		return nil, appErr
	}
	if appErr := assertUserActive(user); appErr != nil {
		return nil, appErr
	}

	result, createErr := s.sessionManager.Create(ctx, session.CreateInput{
		UserID:    user.ID,
		Platform:  input.Platform,
		DeviceID:  input.DeviceID,
		ClientIP:  input.ClientIP,
		UserAgent: input.UserAgent,
	})
	if createErr != nil {
		return nil, createErr
	}
	s.recordLoginAttemptForUser(ctx, user.ID, input, commonYes, "")
	return loginResponseFromToken(result, isNewUser), nil
}

func (s *Service) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	if s == nil || s.sessionManager == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	return s.sessionManager.Refresh(ctx, input)
}

func (s *Service) Logout(ctx context.Context, accessToken string) *apperror.Error {
	if s == nil || s.sessionManager == nil {
		return apperror.Unauthorized("Token认证未配置")
	}
	return s.sessionManager.Logout(ctx, accessToken)
}

func (s *Service) loginByPassword(ctx context.Context, input LoginInput) (*UserCredential, *apperror.Error) {
	if input.Password == "" {
		return nil, apperror.BadRequest("请输入密码")
	}
	if input.CaptchaID == "" || input.CaptchaAnswer == nil {
		return nil, apperror.BadRequest("请完成验证码")
	}
	if s.captchaVerifier == nil {
		return nil, apperror.Internal("验证码服务未配置")
	}
	if appErr := s.captchaVerifier.Verify(ctx, captcha.VerifyInput{
		ID:        input.CaptchaID,
		Answer:    input.CaptchaAnswer,
		ClientIP:  input.ClientIP,
		UserAgent: input.UserAgent,
	}); appErr != nil {
		return nil, appErr
	}

	user, appErr := s.findCredential(ctx, input.LoginAccount)
	if appErr != nil {
		return nil, appErr
	}
	if user == nil {
		return nil, apperror.BadRequest("账号或密码错误")
	}
	if appErr := assertUserActive(user); appErr != nil {
		return nil, appErr
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		return nil, apperror.BadRequest("该账号未设置密码，请使用验证码登录后设置密码")
	}
	if !verifyPassword(input.Password, user.PasswordHash) {
		s.recordLoginAttemptForUser(ctx, user.ID, input, commonNo, "wrong_password")
		return nil, apperror.BadRequest("账号或密码错误")
	}
	return user, nil
}

func (s *Service) loginByCode(ctx context.Context, input LoginInput) (*UserCredential, bool, *apperror.Error) {
	if input.Code == "" {
		return nil, false, apperror.BadRequest("请输入验证码")
	}
	accountType := accountTypeOf(input.LoginAccount)
	if accountType != input.LoginType {
		if input.LoginType == LoginTypeEmail {
			return nil, false, apperror.BadRequest("邮箱格式不正确")
		}
		return nil, false, apperror.BadRequest("手机号格式不正确")
	}
	if appErr := s.verifyCode(ctx, input.LoginAccount, input.Code, VerifyCodeSceneLogin, false); appErr != nil {
		s.recordLoginAttempt(ctx, nil, input, commonNo, "invalid_code")
		return nil, false, appErr
	}

	user, appErr := s.findCredentialByLoginType(ctx, input.LoginAccount, input.LoginType)
	if appErr != nil {
		return nil, false, appErr
	}
	if user != nil {
		if appErr := s.verifyCode(ctx, input.LoginAccount, input.Code, VerifyCodeSceneLogin, true); appErr != nil {
			return nil, false, appErr
		}
		return user, false, nil
	}

	allowed, appErr := s.registerAllowed(ctx, input.Platform)
	if appErr != nil {
		return nil, false, appErr
	}
	if !allowed {
		return nil, false, apperror.BadRequest("暂未开放注册")
	}
	if appErr := s.verifyCode(ctx, input.LoginAccount, input.Code, VerifyCodeSceneLogin, true); appErr != nil {
		return nil, false, appErr
	}
	user, appErr = s.autoRegister(ctx, input.LoginAccount, input.LoginType)
	if appErr != nil {
		return nil, false, appErr
	}
	return user, true, nil
}

func (s *Service) allowedLoginTypes(ctx context.Context, platform string) ([]string, *apperror.Error) {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil, apperror.BadRequest("缺少平台标识")
	}
	if s == nil || s.platformConfig == nil {
		return nil, apperror.Unauthorized("平台策略未配置")
	}
	loginTypes, err := s.platformConfig.LoginTypes(ctx, platform)
	if err != nil {
		return nil, apperror.Internal("平台登录配置查询失败")
	}
	if loginTypes == nil {
		return nil, apperror.BadRequest("无效的平台标识")
	}
	return loginTypes, nil
}

func (s *Service) captchaType(ctx context.Context, platform string) (string, *apperror.Error) {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return "", apperror.BadRequest("缺少平台标识")
	}
	if s == nil || s.platformConfig == nil {
		return "", apperror.Unauthorized("平台策略未配置")
	}
	captchaType, err := s.platformConfig.CaptchaType(ctx, platform)
	if err != nil {
		return "", apperror.Internal("平台验证码配置查询失败")
	}
	if !enum.IsCaptchaType(captchaType) {
		return "", apperror.BadRequest("无效的验证码类型")
	}
	return captchaType, nil
}

func (s *Service) registerAllowed(ctx context.Context, platform string) (bool, *apperror.Error) {
	if s == nil || s.platformConfig == nil {
		return false, apperror.Unauthorized("平台策略未配置")
	}
	allowed, err := s.platformConfig.AllowRegister(ctx, platform)
	if err != nil {
		return false, apperror.Internal("平台注册策略查询失败")
	}
	return allowed, nil
}

func (s *Service) assertLoginTypeAllowed(ctx context.Context, platform string, loginType string) *apperror.Error {
	loginTypes, appErr := s.allowedLoginTypes(ctx, platform)
	if appErr != nil {
		return appErr
	}
	for _, allowed := range loginTypes {
		if allowed == loginType {
			return nil
		}
	}
	return apperror.BadRequest("当前平台不支持该登录方式")
}

func (s *Service) findCredential(ctx context.Context, account string) (*UserCredential, *apperror.Error) {
	return s.findCredentialByLoginType(ctx, account, accountTypeOf(account))
}

func (s *Service) findCredentialByLoginType(ctx context.Context, account string, loginType string) (*UserCredential, *apperror.Error) {
	switch loginType {
	case LoginTypeEmail:
		user, err := s.repository.FindCredentialByEmail(ctx, account)
		if err != nil {
			return nil, apperror.Internal("查询用户失败")
		}
		return user, nil
	case LoginTypePhone:
		user, err := s.repository.FindCredentialByPhone(ctx, account)
		if err != nil {
			return nil, apperror.Internal("查询用户失败")
		}
		return user, nil
	default:
		return nil, apperror.BadRequest("请输入正确的邮箱或手机号")
	}
}

func (s *Service) autoRegister(ctx context.Context, account string, loginType string) (*UserCredential, *apperror.Error) {
	var user *UserCredential
	err := s.repository.WithTx(ctx, func(repo Repository) error {
		role, err := repo.FindDefaultRole(ctx)
		if err != nil {
			return err
		}
		if role == nil || role.ID <= 0 {
			return errDefaultRoleNotFound
		}

		input := CreateUserInput{Username: newUsername(), RoleID: role.ID}
		if loginType == LoginTypeEmail {
			value := account
			input.Email = &value
		} else {
			value := account
			input.Phone = &value
		}
		userID, err := repo.CreateUser(ctx, input)
		if err != nil {
			return err
		}
		if err := repo.CreateProfile(ctx, CreateProfileInput{UserID: userID, Sex: profileSexUnknown}); err != nil {
			return err
		}
		created, err := repo.FindCredentialByID(ctx, userID)
		if err != nil {
			return err
		}
		user = created
		return nil
	})
	if err == nil && user != nil {
		return user, nil
	}
	if errors.Is(err, errDefaultRoleNotFound) {
		return nil, apperror.BadRequest("系统未配置默认角色，无法注册")
	}
	if isDuplicateKey(err) {
		return s.findCredentialByLoginType(ctx, account, loginType)
	}
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "自动注册失败，请稍后重试", err)
	}
	return nil, apperror.Internal("自动注册失败，请稍后重试")
}

func (s *Service) verifyCode(ctx context.Context, account string, code string, scene string, consume bool) *apperror.Error {
	if s == nil || s.codeStore == nil {
		return apperror.Internal("验证码缓存未配置")
	}
	accountType := accountTypeOf(account)
	if accountType == "" {
		return apperror.BadRequest("请输入正确的邮箱或手机号")
	}
	cached, err := s.codeStore.Get(ctx, s.verifyCodeCacheKey(accountType, scene, account))
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "验证码缓存读取失败", err)
	}
	if cached == "" || cached != strings.TrimSpace(code) {
		return apperror.BadRequest("验证码错误或已失效")
	}
	if consume {
		if err := s.codeStore.Delete(ctx, s.verifyCodeCacheKey(accountType, scene, account)); err != nil {
			return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "验证码消费失败", err)
		}
	}
	return nil
}

func (s *Service) verifyCodeCacheKey(accountType string, scene string, account string) string {
	return s.verifyCodeOptions.RedisPrefix + verifyCodeKey(accountType, scene, account)
}

func (s *Service) generateVerifyCode() (string, error) {
	if s.verifyCodeOptions.DevMode {
		return s.verifyCodeOptions.DevCode, nil
	}
	return s.verifyCodeOptions.CodeGenerator()
}

func (s *Service) recordLoginAttemptForUser(ctx context.Context, userID int64, input LoginInput, isSuccess int, reason string) {
	id := userID
	s.recordLoginAttempt(ctx, &id, input, isSuccess, reason)
}

func (s *Service) recordLoginAttempt(ctx context.Context, userID *int64, input LoginInput, isSuccess int, reason string) {
	if s == nil || s.repository == nil {
		return
	}
	attempt := LoginAttempt{
		UserID:       userID,
		LoginAccount: input.LoginAccount,
		LoginType:    input.LoginType,
		Platform:     input.Platform,
		IP:           input.ClientIP,
		UserAgent:    input.UserAgent,
		IsSuccess:    isSuccess,
		Reason:       reason,
	}
	if s.loginLogEnqueuer != nil {
		task, err := NewLoginLogTask(attempt)
		if err != nil {
			s.warnLoginLog(ctx, "build login log task failed, fallback to sync write", err)
		} else if _, err := s.loginLogEnqueuer.Enqueue(ctx, task); err != nil {
			s.warnLoginLog(ctx, "enqueue login log failed, fallback to sync write", err)
		} else {
			return
		}
	}
	if err := s.repository.RecordLoginAttempt(ctx, attempt); err != nil {
		s.warnLoginLog(ctx, "record login log failed", err)
	}
}

func (s *Service) warnLoginLog(ctx context.Context, message string, err error) {
	if s == nil || err == nil {
		return
	}
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(ctx, message, "error", err)
}

func normalizeLoginInput(input LoginInput) LoginInput {
	input.LoginAccount = strings.TrimSpace(input.LoginAccount)
	input.LoginType = strings.TrimSpace(input.LoginType)
	input.Password = strings.TrimSpace(input.Password)
	input.Code = strings.TrimSpace(input.Code)
	input.Platform = strings.TrimSpace(input.Platform)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.ClientIP = strings.TrimSpace(input.ClientIP)
	input.UserAgent = strings.TrimSpace(input.UserAgent)
	return input
}

func normalizeSendCodeInput(input SendCodeInput) SendCodeInput {
	input.Account = strings.TrimSpace(input.Account)
	input.Scene = strings.TrimSpace(input.Scene)
	return input
}

func normalizeVerifyCodeOptions(options VerifyCodeOptions) VerifyCodeOptions {
	if options.TTL <= 0 {
		options.TTL = defaultVerifyCodeTTL
	}
	options.RedisPrefix = strings.TrimSpace(options.RedisPrefix)
	if options.RedisPrefix == "" {
		options.RedisPrefix = defaultVerifyCodeRedisPrefix
	}
	options.DevCode = strings.TrimSpace(options.DevCode)
	if options.DevCode == "" {
		options.DevCode = defaultDevCode
	}
	if options.CodeGenerator == nil {
		options.CodeGenerator = randomSixDigitCode
	}
	return options
}

func assertUserActive(user *UserCredential) *apperror.Error {
	if user == nil {
		return apperror.BadRequest("账号不存在")
	}
	if user.IsDel == commonYes {
		return apperror.BadRequest("账号不存在")
	}
	if user.Status != commonYes {
		return apperror.BadRequest("账号已被禁用，请联系管理员")
	}
	return nil
}

func verifyPassword(password string, hash string) bool {
	hash = strings.TrimSpace(hash)
	if strings.HasPrefix(hash, "$2y$") {
		hash = "$2a$" + strings.TrimPrefix(hash, "$2y$")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func accountTypeOf(value string) string {
	value = strings.TrimSpace(value)
	if emailPattern.MatchString(value) {
		return LoginTypeEmail
	}
	if phonePattern.MatchString(value) {
		return LoginTypePhone
	}
	return ""
}

func loginTypeLabel(loginType string) string {
	switch loginType {
	case LoginTypeEmail:
		return "邮箱登录"
	case LoginTypePhone:
		return "手机号登录"
	case LoginTypePassword:
		return "密码登录"
	default:
		return loginType
	}
}

func loginConfigOrder(loginTypes []string) []string {
	allowed := make(map[string]struct{}, len(loginTypes))
	for _, loginType := range loginTypes {
		if enum.IsLoginType(loginType) {
			allowed[loginType] = struct{}{}
		}
	}

	result := make([]string, 0, len(allowed))
	for _, loginType := range enum.LoginTypes {
		if _, ok := allowed[loginType]; ok {
			result = append(result, loginType)
		}
	}
	return result
}

func randomSixDigitCode() (string, error) {
	max := big.NewInt(900000)
	value, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()+100000), nil
}

func newUsername() string {
	raw := make([]byte, 4)
	if _, err := rand.Read(raw); err != nil {
		return "User_00000000"
	}
	const hexChars = "0123456789abcdef"
	buf := make([]byte, 8)
	for i, b := range raw {
		buf[i*2] = hexChars[b>>4]
		buf[i*2+1] = hexChars[b&0x0f]
	}
	return "User_" + string(buf)
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	return strings.Contains(err.Error(), "Duplicate entry")
}
