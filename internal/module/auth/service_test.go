package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/platform/taskqueue"

	"golang.org/x/crypto/bcrypt"
)

type fakeAuthRepository struct {
	emailQuery   string
	phoneQuery   string
	credential   *UserCredential
	role         *DefaultRole
	created      CreateUserInput
	profile      CreateProfileInput
	passwordID   int64
	passwordHash string
	attempts     []LoginAttempt
	err          error
	txCalled     bool
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

func (f *fakeAuthRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	f.txCalled = true
	return fn(f)
}

func (f *fakeAuthRepository) FindDefaultRole(ctx context.Context) (*DefaultRole, error) {
	return f.role, f.err
}

func (f *fakeAuthRepository) CreateUser(ctx context.Context, input CreateUserInput) (int64, error) {
	f.created = input
	return 99, f.err
}

func (f *fakeAuthRepository) CreateProfile(ctx context.Context, input CreateProfileInput) error {
	f.profile = input
	return f.err
}

func (f *fakeAuthRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	f.passwordID = userID
	f.passwordHash = passwordHash
	return f.err
}

func (f *fakeAuthRepository) FindCredentialByID(ctx context.Context, id int64) (*UserCredential, error) {
	return &UserCredential{ID: id, Status: commonYes, IsDel: commonNo}, f.err
}

type fakeLoginTypeProvider struct {
	types         []string
	captchaType   string
	allowRegister bool
	err           error
}

func (f fakeLoginTypeProvider) LoginTypes(ctx context.Context, platform string) ([]string, error) {
	return f.types, f.err
}

func (f fakeLoginTypeProvider) CaptchaType(ctx context.Context, platform string) (string, error) {
	return f.captchaType, f.err
}

func (f fakeLoginTypeProvider) AllowRegister(ctx context.Context, platform string) (bool, error) {
	return f.allowRegister, f.err
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

type fakeCodeStore struct {
	values  map[string]string
	setKey  string
	setCode string
	setTTL  time.Duration
	deleted string
	err     error
}

func (f *fakeCodeStore) Set(ctx context.Context, key string, code string, ttl time.Duration) error {
	f.setKey = key
	f.setCode = code
	f.setTTL = ttl
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = code
	return f.err
}

func (f *fakeCodeStore) Get(ctx context.Context, key string) (string, error) {
	if f.values == nil {
		return "", f.err
	}
	return f.values[key], f.err
}

func (f *fakeCodeStore) Delete(ctx context.Context, key string) error {
	f.deleted = key
	delete(f.values, key)
	return f.err
}

type fakeLoginLogEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

func (f *fakeLoginLogEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	f.tasks = append(f.tasks, task)
	if f.err != nil {
		return taskqueue.EnqueueResult{}, f.err
	}
	return taskqueue.EnqueueResult{ID: "task-id", Queue: task.Queue, Type: task.Type}, nil
}

func TestServiceLoginConfigReturnsConfiguredLoginTypes(t *testing.T) {
	service := NewService(&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{"email", "phone", "password"}, captchaType: captcha.TypeSlide}, &fakeSessionCreator{}, &fakeCaptchaVerifier{})

	result, appErr := service.LoginConfig(context.Background(), "admin")

	if appErr != nil {
		t.Fatalf("expected login config to succeed, got %v", appErr)
	}
	want := []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}
	if len(result.LoginTypeArr) != len(want) {
		t.Fatalf("expected configured login types %v, got %#v", want, result.LoginTypeArr)
	}
	for i, value := range want {
		if result.LoginTypeArr[i].Value != value {
			t.Fatalf("expected login type %s at index %d, got %#v", value, i, result.LoginTypeArr)
		}
	}
	if !result.CaptchaEnabled || result.CaptchaType != captcha.TypeSlide {
		t.Fatalf("expected slide captcha config, got %#v", result)
	}
}

func TestServiceForgetPasswordConsumesForgetCodeAndWritesPasswordHash(t *testing.T) {
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:forget:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:     42,
		Phone:  "15671628271",
		Status: commonYes,
		IsDel:  commonNo,
	}}
	service := NewService(repo, fakeLoginTypeProvider{}, &fakeSessionCreator{}, &fakeCaptchaVerifier{}, WithCodeStore(store))

	appErr := service.ForgetPassword(context.Background(), ForgetPasswordInput{
		Account:         "15671628271",
		Code:            "123456",
		NewPassword:     "new-secret",
		ConfirmPassword: "new-secret",
	})

	if appErr != nil {
		t.Fatalf("expected forget password to succeed, got %v", appErr)
	}
	if repo.phoneQuery != "15671628271" || repo.passwordID != 42 {
		t.Fatalf("unexpected repository calls: phone=%q passwordID=%d", repo.phoneQuery, repo.passwordID)
	}
	if repo.passwordHash == "" || !strings.HasPrefix(repo.passwordHash, "$2y$") {
		t.Fatalf("expected PHP-compatible bcrypt hash, got %q", repo.passwordHash)
	}
	if !verifyPassword("new-secret", repo.passwordHash) {
		t.Fatalf("expected written hash to verify")
	}
	if store.deleted != "auth:verify_code:phone:forget:d521793014a021c7fec54bb8feee4885" {
		t.Fatalf("expected forget code to be consumed, got %q", store.deleted)
	}
}

func TestServiceForgetPasswordRejectsMismatchedPasswordBeforeConsumingCode(t *testing.T) {
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:forget:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	repo := &fakeAuthRepository{credential: &UserCredential{ID: 42, Phone: "15671628271", Status: commonYes, IsDel: commonNo}}
	service := NewService(repo, fakeLoginTypeProvider{}, &fakeSessionCreator{}, &fakeCaptchaVerifier{}, WithCodeStore(store))

	appErr := service.ForgetPassword(context.Background(), ForgetPasswordInput{
		Account:         "15671628271",
		Code:            "123456",
		NewPassword:     "new-secret",
		ConfirmPassword: "other-secret",
	})

	if appErr == nil || !strings.Contains(appErr.Message, "两次输入的密码不一致") {
		t.Fatalf("expected password mismatch error, got %v", appErr)
	}
	if store.deleted != "" || repo.passwordHash != "" {
		t.Fatalf("mismatch must not consume code or write password: deleted=%q hash=%q", store.deleted, repo.passwordHash)
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

func TestServiceLoginEnqueuesSuccessfulLoginLogWhenProducerConfigured(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	enqueuer := &fakeLoginLogEnqueuer{}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{"password"}},
		&fakeSessionCreator{result: &session.TokenResult{AccessToken: "access-token", RefreshToken: "refresh-token"}},
		&fakeCaptchaVerifier{},
		WithLoginLogEnqueuer(enqueuer),
	)

	_, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount:  "15671628271",
		LoginType:     LoginTypePassword,
		Password:      "123456",
		CaptchaID:     "captcha-id",
		CaptchaAnswer: &captcha.Answer{X: 120, Y: 80},
		Platform:      "admin",
		ClientIP:      "127.0.0.1",
		UserAgent:     "test-agent",
	})

	if appErr != nil {
		t.Fatalf("expected login to succeed, got %v", appErr)
	}
	if len(repo.attempts) != 0 {
		t.Fatalf("expected async queue path instead of sync repository write, got %#v", repo.attempts)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("expected one login log task, got %#v", enqueuer.tasks)
	}
	task := enqueuer.tasks[0]
	if task.Type != TypeAuthLoginLogV1 || task.Queue != taskqueue.QueueCritical {
		t.Fatalf("unexpected login log task metadata: %#v", task)
	}
	attempt, err := DecodeLoginLogPayload(task.Payload)
	if err != nil {
		t.Fatalf("decode login log payload: %v", err)
	}
	if attempt.UserID == nil || *attempt.UserID != 1 || attempt.IsSuccess != commonYes || attempt.Reason != "" {
		t.Fatalf("unexpected login log payload: %#v", attempt)
	}
}

func TestServiceLoginFallsBackToSyncLoginLogWhenEnqueueFails(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	enqueuer := &fakeLoginLogEnqueuer{err: errors.New("redis down")}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{"password"}},
		&fakeSessionCreator{result: &session.TokenResult{AccessToken: "access-token", RefreshToken: "refresh-token"}},
		&fakeCaptchaVerifier{},
		WithLoginLogEnqueuer(enqueuer),
	)

	_, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount:  "15671628271",
		LoginType:     LoginTypePassword,
		Password:      "123456",
		CaptchaID:     "captcha-id",
		CaptchaAnswer: &captcha.Answer{X: 120, Y: 80},
		Platform:      "admin",
	})

	if appErr != nil {
		t.Fatalf("login must not fail because login-log enqueue fails, got %v", appErr)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].IsSuccess != commonYes {
		t.Fatalf("expected sync fallback login log, got %#v", repo.attempts)
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

func TestServiceLoginRejectsWrongPasswordAndEnqueuesFailure(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	enqueuer := &fakeLoginLogEnqueuer{}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{"password"}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithLoginLogEnqueuer(enqueuer),
	)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount:  "15671628271",
		LoginType:     LoginTypePassword,
		Password:      "bad-password",
		CaptchaID:     "captcha-id",
		CaptchaAnswer: &captcha.Answer{X: 120, Y: 80},
		Platform:      "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected wrong password error, got %#v", appErr)
	}
	if len(repo.attempts) != 0 {
		t.Fatalf("expected queue path instead of sync repository write, got %#v", repo.attempts)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("expected one failed login task, got %#v", enqueuer.tasks)
	}
	attempt, err := DecodeLoginLogPayload(enqueuer.tasks[0].Payload)
	if err != nil {
		t.Fatalf("decode login log payload: %v", err)
	}
	if attempt.UserID == nil || *attempt.UserID != 1 || attempt.IsSuccess != commonNo || attempt.Reason != "wrong_password" {
		t.Fatalf("unexpected wrong password payload: %#v", attempt)
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

func TestServiceSendCodeStoresLocalPhoneLoginCode(t *testing.T) {
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: true, DevCode: "123456"}),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{
		Account: "15671628271",
		Scene:   VerifyCodeSceneLogin,
	})

	if appErr != nil {
		t.Fatalf("expected send code to succeed, got %v", appErr)
	}
	if message != "验证码发送成功(测试:123456)" {
		t.Fatalf("unexpected send message %q", message)
	}
	if store.setCode != "123456" || store.setTTL != 5*time.Minute {
		t.Fatalf("unexpected code store write: code=%q ttl=%s", store.setCode, store.setTTL)
	}
	if store.setKey != "auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885" {
		t.Fatalf("unexpected verify code key %q", store.setKey)
	}
}

func TestServicePhoneCodeLoginCreatesNewUserWhenRegisterAllowed(t *testing.T) {
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	repo := &fakeAuthRepository{role: &DefaultRole{ID: 7}}
	sessions := &fakeSessionCreator{result: &session.TokenResult{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{LoginTypePhone}, allowRegister: true},
		sessions,
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: true, DevCode: "123456"}),
	)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Code:         "123456",
		Platform:     "admin",
		DeviceID:     "device-1",
		ClientIP:     "127.0.0.1",
		UserAgent:    "test-agent",
	})

	if appErr != nil {
		t.Fatalf("expected phone code login to succeed, got %v", appErr)
	}
	if result.AccessToken != "access-token" || !result.IsNewUser {
		t.Fatalf("unexpected login result: %#v", result)
	}
	if !repo.txCalled || repo.created.RoleID != 7 || repo.created.Phone == nil || *repo.created.Phone != "15671628271" || repo.created.Email != nil {
		t.Fatalf("unexpected auto register input: tx=%v created=%#v", repo.txCalled, repo.created)
	}
	if repo.profile.UserID != 99 || repo.profile.Sex != 0 {
		t.Fatalf("unexpected profile input: %#v", repo.profile)
	}
	if store.deleted != "auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885" {
		t.Fatalf("expected verify code to be consumed, got deleted key %q", store.deleted)
	}
	if sessions.input.UserID != 99 || sessions.input.Platform != "admin" {
		t.Fatalf("unexpected session input: %#v", sessions.input)
	}
}

func TestServiceCodeLoginRejectsRegisterWhenPlatformDisallowsIt(t *testing.T) {
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	repo := &fakeAuthRepository{role: &DefaultRole{ID: 7}}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{LoginTypePhone}, allowRegister: false},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: true, DevCode: "123456"}),
	)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Code:         "123456",
		Platform:     "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "暂未开放注册" {
		t.Fatalf("expected register disabled error, got %#v", appErr)
	}
	if store.deleted != "" {
		t.Fatalf("verify code must not be consumed when register is denied, got %q", store.deleted)
	}
	if repo.txCalled {
		t.Fatalf("registration transaction should not run when register is denied")
	}
}

func TestServiceCodeLoginRejectsInvalidCodeAndEnqueuesFailure(t *testing.T) {
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "654321",
	}}
	repo := &fakeAuthRepository{}
	enqueuer := &fakeLoginLogEnqueuer{}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{LoginTypePhone}, allowRegister: true},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: true, DevCode: "123456"}),
		WithLoginLogEnqueuer(enqueuer),
	)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Code:         "123456",
		Platform:     "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected invalid code error, got %#v", appErr)
	}
	if len(repo.attempts) != 0 {
		t.Fatalf("expected queue path instead of sync repository write, got %#v", repo.attempts)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("expected one invalid-code login task, got %#v", enqueuer.tasks)
	}
	attempt, err := DecodeLoginLogPayload(enqueuer.tasks[0].Payload)
	if err != nil {
		t.Fatalf("decode login log payload: %v", err)
	}
	if attempt.UserID != nil || attempt.IsSuccess != commonNo || attempt.Reason != "invalid_code" {
		t.Fatalf("unexpected invalid code payload: %#v", attempt)
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

type fakeVerifyCodeMailSender struct {
	scene string
	email string
	code  string
	ttl   time.Duration
	err   *apperror.Error
}

func (f *fakeVerifyCodeMailSender) SendVerifyCode(ctx context.Context, scene string, toEmail string, code string, ttl time.Duration) *apperror.Error {
	f.scene = scene
	f.email = toEmail
	f.code = code
	f.ttl = ttl
	return f.err
}

func TestServiceSendCodeRealEmailUsesMailSender(t *testing.T) {
	store := &fakeCodeStore{}
	mailSender := &fakeVerifyCodeMailSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: false, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{Account: "user@example.com", Scene: VerifyCodeSceneLogin})

	if appErr != nil {
		t.Fatalf("expected real email send code to succeed, got %v", appErr)
	}
	if message != "验证码发送成功" {
		t.Fatalf("unexpected send message %q", message)
	}
	if store.setCode != "654321" || store.setTTL != 5*time.Minute || store.setKey != "auth:verify_code:email:login:b58996c504c5638798eb6b511e6f49af" {
		t.Fatalf("unexpected code store write: key=%q code=%q ttl=%s", store.setKey, store.setCode, store.setTTL)
	}
	if mailSender.scene != VerifyCodeSceneLogin || mailSender.email != "user@example.com" || mailSender.code != "654321" || mailSender.ttl != 5*time.Minute {
		t.Fatalf("unexpected mail sender call: %#v", mailSender)
	}
}

func TestServiceSendCodeRealEmailDeletesCachedCodeWhenMailFails(t *testing.T) {
	store := &fakeCodeStore{}
	mailSender := &fakeVerifyCodeMailSender{err: apperror.Internal("邮件发送失败")}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: false, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{Account: "user@example.com", Scene: VerifyCodeSceneLogin})

	if message != "" || appErr == nil || appErr.Message != "邮件发送失败" {
		t.Fatalf("expected mail failure, message=%q err=%#v", message, appErr)
	}
	if store.deleted != store.setKey || store.values[store.setKey] != "" {
		t.Fatalf("expected cached code cleanup, setKey=%q deleted=%q values=%#v", store.setKey, store.deleted, store.values)
	}
}

func TestServiceSendCodeRealPhoneStillReportsSMSNotConfigured(t *testing.T) {
	store := &fakeCodeStore{}
	mailSender := &fakeVerifyCodeMailSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: false, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{Account: "15671628271", Scene: VerifyCodeSceneLogin})

	if message != "" || appErr == nil || appErr.Message != "短信验证码服务未配置" {
		t.Fatalf("expected SMS not configured, message=%q err=%#v", message, appErr)
	}
	if store.setKey != "" || mailSender.email != "" {
		t.Fatalf("phone real mode must not cache or send email, store=%#v sender=%#v", store, mailSender)
	}
}

func TestServiceSendCodeDevModeStillReturnsTestCode(t *testing.T) {
	store := &fakeCodeStore{}
	mailSender := &fakeVerifyCodeMailSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, DevMode: true, DevCode: "123456"}),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{Account: "user@example.com", Scene: VerifyCodeSceneLogin})

	if appErr != nil || message != "验证码发送成功(测试:123456)" {
		t.Fatalf("expected dev mode test code, message=%q err=%#v", message, appErr)
	}
	if store.setCode != "123456" || mailSender.email != "" {
		t.Fatalf("dev mode must cache test code without real sender, store=%#v sender=%#v", store, mailSender)
	}
}
