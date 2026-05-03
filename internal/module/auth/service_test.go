package auth

import (
	"context"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"

	"golang.org/x/crypto/bcrypt"
)

type fakeAuthRepository struct {
	emailQuery string
	phoneQuery string
	credential *UserCredential
	attempts   []LoginAttempt
	err        error
}

func (f *fakeAuthRepository) FindCredentialByEmail(ctx context.Context, email string) (*UserCredential, error) {
	f.emailQuery = email
	return f.credential, f.err
}

func (f *fakeAuthRepository) FindCredentialByPhone(ctx context.Context, phone string) (*UserCredential, error) {
	f.phoneQuery = phone
	return f.credential, f.err
}

func (f *fakeAuthRepository) RecordLoginAttempt(ctx context.Context, attempt LoginAttempt) error {
	f.attempts = append(f.attempts, attempt)
	return f.err
}

type fakeLoginTypeProvider struct {
	types []string
	err   error
}

func (f fakeLoginTypeProvider) LoginTypes(ctx context.Context, platform string) ([]string, error) {
	return f.types, f.err
}

type fakeSessionCreator struct {
	input  session.CreateInput
	result *session.TokenResult
	err    *apperror.Error
}

func (f *fakeSessionCreator) Create(ctx context.Context, input session.CreateInput) (*session.TokenResult, *apperror.Error) {
	f.input = input
	return f.result, f.err
}

func (f *fakeSessionCreator) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	return nil, f.err
}

func (f *fakeSessionCreator) Logout(ctx context.Context, accessToken string) *apperror.Error {
	return f.err
}

type fakeCaptchaVerifier struct {
	input captcha.VerifyInput
	err   *apperror.Error
}

func (f *fakeCaptchaVerifier) Verify(ctx context.Context, input captcha.VerifyInput) *apperror.Error {
	f.input = input
	return f.err
}

func TestServiceLoginConfigReturnsImplementedPasswordOnly(t *testing.T) {
	service := NewService(&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{"email", "phone", "password"}}, &fakeSessionCreator{}, &fakeCaptchaVerifier{})

	result, appErr := service.LoginConfig(context.Background(), "admin")

	if appErr != nil {
		t.Fatalf("expected login config to succeed, got %v", appErr)
	}
	if len(result.LoginTypeArr) != 1 || result.LoginTypeArr[0].Value != LoginTypePassword {
		t.Fatalf("expected password-only config, got %#v", result.LoginTypeArr)
	}
	if !result.CaptchaEnabled || result.CaptchaType != captcha.TypeSlide {
		t.Fatalf("expected slide captcha config, got %#v", result)
	}
}

func TestServiceLoginVerifiesPHPBcryptPasswordAndCreatesSession(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	sessions := &fakeSessionCreator{result: &session.TokenResult{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	captchaVerifier := &fakeCaptchaVerifier{}
	service := NewService(repo, fakeLoginTypeProvider{types: []string{"password"}}, sessions, captchaVerifier)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		CaptchaID:    "captcha-id",
		CaptchaAnswer: &captcha.Answer{
			X: 120,
			Y: 80,
		},
		Platform:  "admin",
		DeviceID:  "device-1",
		ClientIP:  "127.0.0.1",
		UserAgent: "test-agent",
	})

	if appErr != nil {
		t.Fatalf("expected login to succeed, got %v", appErr)
	}
	if result.AccessToken != "access-token" || result.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected token result: %#v", result)
	}
	if repo.phoneQuery != "15671628271" {
		t.Fatalf("expected phone lookup, got %q", repo.phoneQuery)
	}
	if captchaVerifier.input.ID != "captcha-id" || captchaVerifier.input.Answer == nil ||
		captchaVerifier.input.Answer.X != 120 || captchaVerifier.input.Answer.Y != 80 {
		t.Fatalf("expected captcha verification, got %#v", captchaVerifier.input)
	}
	if sessions.input.UserID != 1 || sessions.input.Platform != "admin" || sessions.input.DeviceID != "device-1" {
		t.Fatalf("unexpected session create input: %#v", sessions.input)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].IsSuccess != commonYes || repo.attempts[0].UserID == nil || *repo.attempts[0].UserID != 1 {
		t.Fatalf("expected successful login attempt log, got %#v", repo.attempts)
	}
}

func TestServiceLoginRejectsWrongPasswordAndLogsFailure(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	captchaVerifier := &fakeCaptchaVerifier{}
	service := NewService(repo, fakeLoginTypeProvider{types: []string{"password"}}, &fakeSessionCreator{}, captchaVerifier)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "bad-password",
		CaptchaID:    "captcha-id",
		CaptchaAnswer: &captcha.Answer{
			X: 120,
			Y: 80,
		},
		Platform: "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "账号或密码错误" {
		t.Fatalf("expected wrong password error, got %#v", appErr)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].IsSuccess != commonNo || repo.attempts[0].Reason != "wrong_password" {
		t.Fatalf("expected failed login attempt log, got %#v", repo.attempts)
	}
	if captchaVerifier.input.ID != "captcha-id" {
		t.Fatalf("expected captcha to be verified before password check, got %#v", captchaVerifier.input)
	}
}

func TestServiceLoginRejectsMissingCaptchaBeforeCredentialLookup(t *testing.T) {
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: phpBcryptHash(t, "123456"),
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	service := NewService(repo, fakeLoginTypeProvider{types: []string{"password"}}, &fakeSessionCreator{}, &fakeCaptchaVerifier{})

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		Platform:     "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "请完成验证码" {
		t.Fatalf("expected missing captcha error, got %#v", appErr)
	}
	if repo.phoneQuery != "" {
		t.Fatalf("expected credential lookup to be skipped, got %q", repo.phoneQuery)
	}
}

func TestServiceLoginRejectsInvalidCaptchaBeforeCredentialLookup(t *testing.T) {
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: phpBcryptHash(t, "123456"),
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	service := NewService(repo, fakeLoginTypeProvider{types: []string{"password"}}, &fakeSessionCreator{}, &fakeCaptchaVerifier{
		err: apperror.BadRequest("验证码错误或已过期"),
	})

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		CaptchaID:    "captcha-id",
		CaptchaAnswer: &captcha.Answer{
			X: 40,
			Y: 80,
		},
		Platform: "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "验证码错误或已过期" {
		t.Fatalf("expected invalid captcha error, got %#v", appErr)
	}
	if repo.phoneQuery != "" {
		t.Fatalf("expected credential lookup to be skipped, got %q", repo.phoneQuery)
	}
}

func phpBcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate bcrypt hash: %v", err)
	}
	return strings.Replace(string(hash), "$2a$", "$2y$", 1)
}
