package auth

import (
	"context"
	"net/mail"
	"regexp"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"

	"golang.org/x/crypto/bcrypt"
)

const (
	LoginTypeEmail    = "email"
	LoginTypePhone    = "phone"
	LoginTypePassword = "password"
)

var phonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

type LoginTypeProvider interface {
	LoginTypes(ctx context.Context, platform string) ([]string, error)
}

type SessionManager interface {
	Create(ctx context.Context, input session.CreateInput) (*session.TokenResult, *apperror.Error)
	Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error)
	Logout(ctx context.Context, accessToken string) *apperror.Error
}

type CaptchaVerifier interface {
	Verify(ctx context.Context, input captcha.VerifyInput) *apperror.Error
}

type Service struct {
	repository      Repository
	loginTypes      LoginTypeProvider
	sessionManager  SessionManager
	captchaVerifier CaptchaVerifier
}

func NewService(repository Repository, loginTypes LoginTypeProvider, sessionManager SessionManager, captchaVerifier CaptchaVerifier) *Service {
	return &Service{
		repository:      repository,
		loginTypes:      loginTypes,
		sessionManager:  sessionManager,
		captchaVerifier: captchaVerifier,
	}
}

func (s *Service) LoginConfig(ctx context.Context, platform string) (*LoginConfigResponse, *apperror.Error) {
	loginTypes, appErr := s.allowedLoginTypes(ctx, platform)
	if appErr != nil {
		return nil, appErr
	}
	options := make([]LoginTypeOption, 0, len(loginTypes))
	for _, loginType := range loginTypes {
		if loginType == LoginTypePassword {
			options = append(options, LoginTypeOption{Label: loginTypeLabel(loginType), Value: loginType})
		}
	}
	return &LoginConfigResponse{
		LoginTypeArr:   options,
		CaptchaEnabled: true,
		CaptchaType:    captcha.TypeSlide,
	}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*session.TokenResult, *apperror.Error) {
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
	if input.LoginType != LoginTypePassword {
		return nil, apperror.BadRequest("验证码登录暂未迁移")
	}
	if input.Password == "" {
		return nil, apperror.BadRequest("请输入密码")
	}

	if appErr := s.assertLoginTypeAllowed(ctx, input.Platform, input.LoginType); appErr != nil {
		return nil, appErr
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
	if user.IsDel == commonYes {
		return nil, apperror.BadRequest("账号不存在")
	}
	if user.Status != commonYes {
		return nil, apperror.BadRequest("账号已被禁用，请联系管理员")
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		return nil, apperror.BadRequest("该账号未设置密码，请使用验证码登录后设置密码")
	}
	if !verifyPassword(input.Password, user.PasswordHash) {
		s.recordLoginAttempt(ctx, user.ID, input, commonNo, "wrong_password")
		return nil, apperror.BadRequest("账号或密码错误")
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
	s.recordLoginAttempt(ctx, user.ID, input, commonYes, "")
	return result, nil
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

func (s *Service) allowedLoginTypes(ctx context.Context, platform string) ([]string, *apperror.Error) {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil, apperror.BadRequest("缺少平台标识")
	}
	if s == nil || s.loginTypes == nil {
		return nil, apperror.Unauthorized("平台策略未配置")
	}
	loginTypes, err := s.loginTypes.LoginTypes(ctx, platform)
	if err != nil {
		return nil, apperror.Internal("平台登录配置查询失败")
	}
	if loginTypes == nil {
		return nil, apperror.BadRequest("无效的平台标识")
	}
	return loginTypes, nil
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
	if isEmail(account) {
		user, err := s.repository.FindCredentialByEmail(ctx, account)
		if err != nil {
			return nil, apperror.Internal("查询用户失败")
		}
		return user, nil
	}
	if phonePattern.MatchString(account) {
		user, err := s.repository.FindCredentialByPhone(ctx, account)
		if err != nil {
			return nil, apperror.Internal("查询用户失败")
		}
		return user, nil
	}
	return nil, apperror.BadRequest("请输入正确的邮箱或手机号")
}

func (s *Service) recordLoginAttempt(ctx context.Context, userID int64, input LoginInput, isSuccess int, reason string) {
	if s == nil || s.repository == nil {
		return
	}
	id := userID
	_ = s.repository.RecordLoginAttempt(ctx, LoginAttempt{
		UserID:       &id,
		LoginAccount: input.LoginAccount,
		LoginType:    input.LoginType,
		Platform:     input.Platform,
		IP:           input.ClientIP,
		UserAgent:    input.UserAgent,
		IsSuccess:    isSuccess,
		Reason:       reason,
	})
}

func normalizeLoginInput(input LoginInput) LoginInput {
	input.LoginAccount = strings.TrimSpace(input.LoginAccount)
	input.LoginType = strings.TrimSpace(input.LoginType)
	input.Password = strings.TrimSpace(input.Password)
	input.Platform = strings.TrimSpace(input.Platform)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.ClientIP = strings.TrimSpace(input.ClientIP)
	input.UserAgent = strings.TrimSpace(input.UserAgent)
	return input
}

func verifyPassword(password string, hash string) bool {
	hash = strings.TrimSpace(hash)
	if strings.HasPrefix(hash, "$2y$") {
		hash = "$2a$" + strings.TrimPrefix(hash, "$2y$")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func isEmail(value string) bool {
	_, err := mail.ParseAddress(value)
	return err == nil && strings.Contains(value, "@")
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
