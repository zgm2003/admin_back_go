package auth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"

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
	input  CreateInput
	result *TokenResult
	err    *apperror.Error
}

func (f *fakeSessionCreator) Issue(ctx context.Context, input IssueCommand) (*CredentialSet, *apperror.Error) {
	return f.Create(ctx, input)
}

func (f *fakeSessionCreator) Authenticate(context.Context, AccessCredential) (*Identity, *apperror.Error) {
	return nil, nil
}

func (f *fakeSessionCreator) Rotate(ctx context.Context, input RotateCommand) (*CredentialSet, *apperror.Error) {
	return f.Refresh(ctx, input)
}

func (f *fakeSessionCreator) Revoke(ctx context.Context, command RevokeCommand) *apperror.Error {
	return f.Logout(ctx, command.AccessToken)
}

func (f *fakeSessionCreator) Create(ctx context.Context, input CreateInput) (*TokenResult, *apperror.Error) {
	f.input = input
	return f.result, f.err
}

func (f *fakeSessionCreator) Refresh(ctx context.Context, input RefreshInput) (*TokenResult, *apperror.Error) {
	return nil, f.err
}

func (f *fakeSessionCreator) Logout(ctx context.Context, accessToken string) *apperror.Error {
	return f.err
}

type fakeCaptchaVerifier struct {
	input VerifyInput
	err   *apperror.Error
	calls int
}

type verifyCodeReadinessCall struct {
	accountType string
	scene       string
}

type fakeVerifyCodeReadinessProvider struct {
	readyByAccountType map[string]bool
	err                *apperror.Error
	calls              []verifyCodeReadinessCall
}

func (f *fakeVerifyCodeReadinessProvider) VerifyCodeReady(_ context.Context, accountType, scene string) (bool, *apperror.Error) {
	f.calls = append(f.calls, verifyCodeReadinessCall{accountType: accountType, scene: scene})
	if f.err != nil {
		return false, f.err
	}
	return f.readyByAccountType[accountType], nil
}

func allVerificationChannelsReady() *fakeVerifyCodeReadinessProvider {
	return &fakeVerifyCodeReadinessProvider{readyByAccountType: map[string]bool{
		LoginTypeEmail: true,
		LoginTypePhone: true,
	}}
}

func (f *fakeCaptchaVerifier) Verify(ctx context.Context, input VerifyInput) *apperror.Error {
	f.calls++
	f.input = input
	return f.err
}

func validLoginSendCodeInput(account string, loginType string) SendCodeInput {
	return SendCodeInput{
		Account:       account,
		Scene:         VerifyCodeSceneLogin,
		LoginType:     loginType,
		CaptchaID:     "captcha-id",
		CaptchaAnswer: &Answer{X: 120, Y: 80},
		ClientIP:      "127.0.0.1",
		UserAgent:     "test-agent",
	}
}

type fakeCodeStore struct {
	values  map[string]string
	setKey  string
	setCode string
	setTTL  time.Duration
	getKey  string
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
	f.getKey = key
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
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{"email", "phone", "password"}, captchaType: TypeSlide, allowRegister: true},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithVerifyCodeReadinessProvider(&fakeVerifyCodeReadinessProvider{readyByAccountType: map[string]bool{
			LoginTypeEmail: true,
			LoginTypePhone: true,
		}}),
	)

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
	if !result.CaptchaEnabled || result.CaptchaType != TypeSlide {
		t.Fatalf("expected slide captcha config, got %#v", result)
	}
	if !result.AllowRegister {
		t.Fatalf("expected login-config to expose allow_register=true, got %#v", result)
	}
}

func TestServiceLoginConfigFiltersUnavailableVerificationChannels(t *testing.T) {
	for _, tt := range []struct {
		name  string
		ready map[string]bool
		want  []string
	}{
		{name: "both unavailable", ready: map[string]bool{}, want: []string{LoginTypePassword}},
		{name: "email ready", ready: map[string]bool{LoginTypeEmail: true}, want: []string{LoginTypeEmail, LoginTypePassword}},
		{name: "phone ready", ready: map[string]bool{LoginTypePhone: true}, want: []string{LoginTypePhone, LoginTypePassword}},
		{name: "both ready", ready: map[string]bool{LoginTypeEmail: true, LoginTypePhone: true}, want: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness := &fakeVerifyCodeReadinessProvider{readyByAccountType: tt.ready}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: []string{LoginTypePhone, LoginTypePassword, LoginTypeEmail}, captchaType: TypeSlide},
				&fakeSessionCreator{},
				&fakeCaptchaVerifier{},
				WithVerifyCodeReadinessProvider(readiness),
			)
			result, appErr := service.LoginConfig(context.Background(), "admin")
			if appErr != nil {
				t.Fatalf("LoginConfig error=%#v", appErr)
			}
			if len(result.LoginTypeArr) != len(tt.want) {
				t.Fatalf("types=%#v", result.LoginTypeArr)
			}
			for i, want := range tt.want {
				if result.LoginTypeArr[i].Value != want {
					t.Fatalf("types=%#v", result.LoginTypeArr)
				}
			}
			for _, call := range readiness.calls {
				if call.scene != VerifyCodeSceneLogin {
					t.Fatalf("calls=%#v", readiness.calls)
				}
			}
		})
	}
}

func TestServiceLoginConfigPropagatesReadinessFailure(t *testing.T) {
	wantErr := apperror.Internal("mail repository unavailable")
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePassword}, captchaType: TypeSlide},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithVerifyCodeReadinessProvider(&fakeVerifyCodeReadinessProvider{err: wantErr}),
	)
	result, appErr := service.LoginConfig(context.Background(), "admin")
	if result != nil || appErr != wantErr {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
}

func TestServiceLoginConfigQueriesOnlyConfiguredVerificationChannels(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configured []string
		want       []string
		wantCalls  []verifyCodeReadinessCall
	}{
		{name: "password only", configured: []string{LoginTypePassword}, want: []string{LoginTypePassword}},
		{
			name:       "email and password",
			configured: []string{LoginTypePassword, LoginTypeEmail},
			want:       []string{LoginTypeEmail, LoginTypePassword},
			wantCalls:  []verifyCodeReadinessCall{{accountType: LoginTypeEmail, scene: VerifyCodeSceneLogin}},
		},
		{
			name:       "phone and password",
			configured: []string{LoginTypePassword, LoginTypePhone},
			want:       []string{LoginTypePhone, LoginTypePassword},
			wantCalls:  []verifyCodeReadinessCall{{accountType: LoginTypePhone, scene: VerifyCodeSceneLogin}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness := allVerificationChannelsReady()
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: tt.configured, captchaType: TypeSlide},
				&fakeSessionCreator{},
				&fakeCaptchaVerifier{},
				WithVerifyCodeReadinessProvider(readiness),
			)
			result, appErr := service.LoginConfig(context.Background(), "admin")
			if appErr != nil {
				t.Fatalf("LoginConfig error=%#v", appErr)
			}
			if len(result.LoginTypeArr) != len(tt.want) {
				t.Fatalf("types=%#v", result.LoginTypeArr)
			}
			for i, want := range tt.want {
				if result.LoginTypeArr[i].Value != want {
					t.Fatalf("types=%#v", result.LoginTypeArr)
				}
			}
			if len(readiness.calls) != len(tt.wantCalls) {
				t.Fatalf("calls=%#v want=%#v", readiness.calls, tt.wantCalls)
			}
			for i, want := range tt.wantCalls {
				if readiness.calls[i] != want {
					t.Fatalf("calls=%#v want=%#v", readiness.calls, tt.wantCalls)
				}
			}
		})
	}
}

func TestServiceLoginConfigReturnsRegisterSwitch(t *testing.T) {
	service := NewService(&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{"password"}, captchaType: TypeSlide, allowRegister: false}, &fakeSessionCreator{}, &fakeCaptchaVerifier{})

	result, appErr := service.LoginConfig(context.Background(), "canvas")

	if appErr != nil {
		t.Fatalf("expected login config to succeed, got %v", appErr)
	}
	if result.AllowRegister {
		t.Fatalf("expected login-config to expose allow_register=false, got %#v", result)
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

func TestServiceLoginVerifiesPHPBcryptPasswordWithoutCaptchaAndCreatesSession(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	sessions := &fakeSessionCreator{result: &TokenResult{
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
		Platform:     "admin",
		DeviceID:     "device-1",
		ClientIP:     "127.0.0.1",
		UserAgent:    "test-agent",
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
	if captchaVerifier.calls != 0 {
		t.Fatalf("password login must not invoke captcha verification, got %d calls", captchaVerifier.calls)
	}
	if sessions.input.UserID != 1 || sessions.input.Platform != "admin" || sessions.input.DeviceID != "device-1" {
		t.Fatalf("unexpected session create input: %#v", sessions.input)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].IsSuccess != commonYes || repo.attempts[0].UserID == nil || *repo.attempts[0].UserID != 1 {
		t.Fatalf("expected successful login attempt log, got %#v", repo.attempts)
	}
}

func TestServicePasswordLoginDoesNotQueryVerificationReadiness(t *testing.T) {
	readiness := &fakeVerifyCodeReadinessProvider{err: apperror.Internal("channel repository unavailable")}
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           1,
		PasswordHash: phpBcryptHash(t, "123456"),
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}},
		&fakeSessionCreator{result: &TokenResult{AccessToken: "access-token"}},
		&fakeCaptchaVerifier{},
		WithVerifyCodeReadinessProvider(readiness),
	)
	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		Platform:     "admin",
	})
	if appErr != nil || result == nil || result.AccessToken != "access-token" {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
	if len(readiness.calls) != 0 {
		t.Fatalf("calls=%#v", readiness.calls)
	}
}

func TestServiceAppPasswordLoginRequiresCaptcha(t *testing.T) {
	service := NewService(&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{"password"}}, &fakeSessionCreator{}, &fakeCaptchaVerifier{})

	_, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		Platform:     "app",
	})

	if appErr == nil || !strings.Contains(appErr.Message, "请完成验证码") {
		t.Fatalf("expected app password login to require captcha, got %v", appErr)
	}
}

func TestServiceAppPasswordLoginVerifiesCaptchaAndReturnsUserID(t *testing.T) {
	hash := phpBcryptHash(t, "123456")
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID:           7,
		PasswordHash: hash,
		Status:       commonYes,
		IsDel:        commonNo,
	}}
	sessions := &fakeSessionCreator{result: &TokenResult{
		AccessToken: "app-token",
		ExpiresIn:   14400,
	}}
	captchaVerifier := &fakeCaptchaVerifier{}
	service := NewService(repo, fakeLoginTypeProvider{types: []string{"password"}}, sessions, captchaVerifier)

	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		CaptchaID:    "captcha-id",
		CaptchaAnswer: &Answer{
			X: 120,
			Y: 80,
		},
		Platform: "app",
		DeviceID: "ios-1",
	})

	if appErr != nil {
		t.Fatalf("expected app login to succeed with captcha, got %v", appErr)
	}
	if result.AccessToken != "app-token" || result.UserID != 7 {
		t.Fatalf("unexpected app login result: %#v", result)
	}
	if captchaVerifier.input.ID != "captcha-id" || captchaVerifier.input.Answer == nil ||
		captchaVerifier.input.Answer.X != 120 || captchaVerifier.input.Answer.Y != 80 {
		t.Fatalf("expected app password login to verify captcha, got %#v", captchaVerifier.input)
	}
	if sessions.input.UserID != 7 || sessions.input.Platform != "app" || sessions.input.DeviceID != "ios-1" {
		t.Fatalf("unexpected app session input: %#v", sessions.input)
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
		&fakeSessionCreator{result: &TokenResult{AccessToken: "access-token", RefreshToken: "refresh-token"}},
		&fakeCaptchaVerifier{},
		WithLoginLogEnqueuer(enqueuer),
	)

	_, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		Platform:     "admin",
		ClientIP:     "127.0.0.1",
		UserAgent:    "test-agent",
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
	if task.Type != TypeAuthLoginLogV1 || task.Queue != "" {
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
		&fakeSessionCreator{result: &TokenResult{AccessToken: "access-token", RefreshToken: "refresh-token"}},
		&fakeCaptchaVerifier{},
		WithLoginLogEnqueuer(enqueuer),
	)

	_, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "123456",
		Platform:     "admin",
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
		Platform:     "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "账号或密码错误" {
		t.Fatalf("expected wrong password error, got %#v", appErr)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].IsSuccess != commonNo || repo.attempts[0].Reason != "wrong_password" {
		t.Fatalf("expected failed login attempt log, got %#v", repo.attempts)
	}
	if captchaVerifier.calls != 0 {
		t.Fatalf("admin password login must not invoke captcha verification, got %d calls", captchaVerifier.calls)
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
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Password:     "bad-password",
		Platform:     "admin",
	})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
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

func TestServiceSendCodeGeneratesCachesAndSendsPhoneVerification(t *testing.T) {
	store := &fakeCodeStore{}
	phoneSender := &fakeVerifyCodePhoneSender{}
	generatorCalls := 0
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(phoneSender),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodePolicyProvider(&fakeVerifyCodePolicyProvider{ttlByAccountType: map[string]time.Duration{LoginTypePhone: 9 * time.Minute}}),
		WithVerifyCodeOptions(VerifyCodeOptions{CodeGenerator: func() (string, error) {
			generatorCalls++
			return "654321", nil
		}}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))

	if appErr != nil {
		t.Fatalf("expected send code to succeed, got %v", appErr)
	}
	if message != "验证码发送成功" {
		t.Fatalf("unexpected send message %q", message)
	}
	if store.setCode != "654321" || store.setTTL != 9*time.Minute {
		t.Fatalf("unexpected code store write: code=%q ttl=%s", store.setCode, store.setTTL)
	}
	if store.setKey != "auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885" {
		t.Fatalf("unexpected verify code key %q", store.setKey)
	}
	if phoneSender.scene != VerifyCodeSceneLogin || phoneSender.phone != "15671628271" || phoneSender.code != store.setCode || phoneSender.ttl != store.setTTL {
		t.Fatalf("sender=%#v store=%#v", phoneSender, store)
	}
	if generatorCalls != 1 {
		t.Fatalf("generator calls=%d", generatorCalls)
	}
}

func TestServiceSendCodeValidatesInputBeforeCodeStoreDependency(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   SendCodeInput
		message string
	}{
		{name: "empty account", input: SendCodeInput{Scene: VerifyCodeSceneLogin}, message: "账号不能为空"},
		{name: "invalid account", input: SendCodeInput{Account: "not-an-account", Scene: VerifyCodeSceneLogin}, message: "请输入正确的邮箱或手机号"},
		{name: "invalid scene", input: SendCodeInput{Account: "15671628271", Scene: "unknown"}, message: "无效的验证码场景"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&fakeAuthRepository{}, fakeLoginTypeProvider{}, &fakeSessionCreator{}, &fakeCaptchaVerifier{})
			message, appErr := service.SendCode(context.Background(), tt.input)
			if message != "" || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != tt.message {
				t.Fatalf("message=%q err=%#v", message, appErr)
			}
		})
	}
}

func TestServiceSendCodeRejectsUnavailableChannelBeforeCaptcha(t *testing.T) {
	for _, tt := range []struct {
		name        string
		account     string
		accountType string
		scene       string
		loginType   string
	}{
		{name: "phone login", account: "15671628271", accountType: LoginTypePhone, scene: VerifyCodeSceneLogin, loginType: LoginTypePhone},
		{name: "email bind", account: "user@example.com", accountType: LoginTypeEmail, scene: VerifyCodeSceneBindEmail},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &fakeCaptchaVerifier{}
			store := &fakeCodeStore{}
			readiness := &fakeVerifyCodeReadinessProvider{readyByAccountType: map[string]bool{}}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: []string{tt.loginType}},
				&fakeSessionCreator{},
				verifier,
				WithCodeStore(store),
				WithVerifyCodeReadinessProvider(readiness),
			)
			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account:       tt.account,
				Scene:         tt.scene,
				LoginType:     tt.loginType,
				CaptchaID:     "captcha-id",
				CaptchaAnswer: &Answer{X: 120, Y: 80},
			})
			if message != "" || appErr == nil || appErr.Code != "auth.verify_code.channel_unavailable" {
				t.Fatalf("message=%q err=%#v", message, appErr)
			}
			if verifier.calls != 0 || store.setKey != "" {
				t.Fatalf("verifier=%#v store=%#v", verifier, store)
			}
			if len(readiness.calls) != 1 || readiness.calls[0].accountType != tt.accountType || readiness.calls[0].scene != tt.scene {
				t.Fatalf("calls=%#v", readiness.calls)
			}
		})
	}
}

func TestServiceSendCodePropagatesReadinessFailureBeforeCaptcha(t *testing.T) {
	wantErr := apperror.Internal("sms repository unavailable")
	readiness := &fakeVerifyCodeReadinessProvider{err: wantErr}
	verifier := &fakeCaptchaVerifier{}
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		verifier,
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(readiness),
	)
	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))
	if message != "" || appErr != wantErr {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	wantCall := verifyCodeReadinessCall{accountType: LoginTypePhone, scene: VerifyCodeSceneLogin}
	if len(readiness.calls) != 1 || readiness.calls[0] != wantCall {
		t.Fatalf("calls=%#v", readiness.calls)
	}
	if verifier.calls != 0 || store.setKey != "" || store.getKey != "" || store.deleted != "" {
		t.Fatalf("verifier=%#v store=%#v", verifier, store)
	}
}

func TestServiceSendCodeRejectsBindSceneAccountMismatchBeforeReadiness(t *testing.T) {
	for _, tt := range []struct {
		name          string
		account       string
		scene         string
		wantMessageID string
	}{
		{name: "phone account for bind email", account: "15671628271", scene: VerifyCodeSceneBindEmail, wantMessageID: "auth.send_code.email.invalid"},
		{name: "email account for bind phone", account: "user@example.com", scene: VerifyCodeSceneBindPhone, wantMessageID: "auth.send_code.phone.invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness := allVerificationChannelsReady()
			verifier := &fakeCaptchaVerifier{}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{},
				&fakeSessionCreator{},
				verifier,
				WithCodeStore(&fakeCodeStore{}),
				WithVerifyCodeReadinessProvider(readiness),
			)
			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account:       tt.account,
				Scene:         tt.scene,
				CaptchaID:     "captcha-id",
				CaptchaAnswer: &Answer{X: 120, Y: 80},
			})
			if message != "" || appErr == nil || appErr.MessageID != tt.wantMessageID {
				t.Fatalf("message=%q err=%#v", message, appErr)
			}
			if len(readiness.calls) != 0 || verifier.calls != 0 {
				t.Fatalf("readiness=%#v verifier=%#v", readiness.calls, verifier)
			}
		})
	}
}

func TestServiceSendCodeRejectsLoginRequestWithoutSecurityProof(t *testing.T) {
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{
		Account: "15671628271",
		Scene:   VerifyCodeSceneLogin,
	})

	if message != "" || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected login send-code without security proof to be rejected, message=%q err=%#v", message, appErr)
	}
	if store.setKey != "" {
		t.Fatalf("mock phone code must not be cached before security proof passes: %#v", store)
	}
}

func TestServiceSendCodeLoginRequiresCaptchaForEmailAndPhone(t *testing.T) {
	tests := []struct {
		name      string
		account   string
		loginType string
	}{
		{name: "email", account: "user@example.com", loginType: LoginTypeEmail},
		{name: "phone", account: "15671628271", loginType: LoginTypePhone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCodeStore{}
			verifier := &fakeCaptchaVerifier{}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: []string{tt.loginType}},
				&fakeSessionCreator{},
				verifier,
				WithCodeStore(store),
				WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
				WithVerifyCodeMailSender(&fakeVerifyCodeMailSender{}),
			)

			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account:   tt.account,
				Scene:     VerifyCodeSceneLogin,
				LoginType: tt.loginType,
			})

			if message != "" || appErr == nil || appErr.MessageID != "captcha.required" {
				t.Fatalf("expected captcha.required, message=%q err=%#v", message, appErr)
			}
			if verifier.input.ID != "" || store.setKey != "" {
				t.Fatalf("missing captcha must stop before verification and caching: verifier=%#v store=%#v", verifier.input, store)
			}
		})
	}
}

func TestServiceSendCodeRequiresCaptchaForEveryScene(t *testing.T) {
	tests := []struct {
		name      string
		account   string
		scene     string
		loginType string
	}{
		{name: "login", account: "15671628271", scene: VerifyCodeSceneLogin, loginType: LoginTypePhone},
		{name: "forget", account: "15671628271", scene: VerifyCodeSceneForget},
		{name: "bind phone", account: "15671628271", scene: VerifyCodeSceneBindPhone},
		{name: "bind email", account: "user@example.com", scene: VerifyCodeSceneBindEmail},
		{name: "change password", account: "15671628271", scene: VerifyCodeSceneChangePassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCodeStore{}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{},
				&fakeSessionCreator{},
				&fakeCaptchaVerifier{},
				WithCodeStore(store),
				WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
				WithVerifyCodeMailSender(&fakeVerifyCodeMailSender{}),
			)

			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account:   tt.account,
				Scene:     tt.scene,
				LoginType: tt.loginType,
			})

			if message != "" || appErr == nil || appErr.MessageID != "captcha.required" || appErr.Code != "captcha.required" {
				t.Fatalf("scene=%s expected captcha.required, message=%q err=%#v", tt.scene, message, appErr)
			}
			if store.setKey != "" {
				t.Fatalf("scene=%s must not cache a code before captcha passes: %#v", tt.scene, store)
			}
		})
	}
}

func TestServiceSendCodeLoginStopsWhenCaptchaIsRejected(t *testing.T) {
	store := &fakeCodeStore{}
	verifier := &fakeCaptchaVerifier{err: apperror.BadRequestKey("captcha.invalid_or_expired", nil, "验证码错误或已过期")}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		verifier,
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
	)

	message, appErr := service.SendCode(context.Background(), SendCodeInput{
		Account:       "15671628271",
		Scene:         VerifyCodeSceneLogin,
		LoginType:     LoginTypePhone,
		CaptchaID:     "captcha-id",
		CaptchaAnswer: &Answer{X: 120, Y: 80},
		ClientIP:      "127.0.0.1",
		UserAgent:     "test-agent",
	})

	if message != "" || appErr == nil || appErr.MessageID != "captcha.invalid_or_expired" {
		t.Fatalf("expected rejected captcha error, message=%q err=%#v", message, appErr)
	}
	if verifier.input.ID != "captcha-id" || verifier.input.Answer == nil || verifier.input.Answer.X != 120 || verifier.input.Answer.Y != 80 || verifier.input.ClientIP != "127.0.0.1" || verifier.input.UserAgent != "test-agent" {
		t.Fatalf("unexpected captcha verification input: %#v", verifier.input)
	}
	if store.setKey != "" {
		t.Fatalf("mock phone code must not be cached after captcha rejection: %#v", store)
	}
}

func TestServiceSendCodeLoginVerifiesCaptchaBeforeSendingEmailOrPhone(t *testing.T) {
	tests := []struct {
		name      string
		account   string
		loginType string
	}{
		{name: "email", account: "user@example.com", loginType: LoginTypeEmail},
		{name: "phone", account: "15671628271", loginType: LoginTypePhone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCodeStore{}
			verifier := &fakeCaptchaVerifier{}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: []string{tt.loginType}},
				&fakeSessionCreator{},
				verifier,
				WithCodeStore(store),
				WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
				WithVerifyCodeMailSender(&fakeVerifyCodeMailSender{}),
				WithVerifyCodePhoneSender(&fakeVerifyCodePhoneSender{}),
				WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
			)

			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account:       tt.account,
				Scene:         VerifyCodeSceneLogin,
				LoginType:     tt.loginType,
				CaptchaID:     "captcha-id",
				CaptchaAnswer: &Answer{X: 120, Y: 80},
				ClientIP:      "127.0.0.1",
				UserAgent:     "test-agent",
			})

			if appErr != nil || message != "验证码发送成功" {
				t.Fatalf("expected send-code success, message=%q err=%#v", message, appErr)
			}
			if verifier.input.ID != "captcha-id" || verifier.input.Answer == nil || verifier.input.ClientIP != "127.0.0.1" || verifier.input.UserAgent != "test-agent" {
				t.Fatalf("captcha must be verified before sending: %#v", verifier.input)
			}
			if store.setKey == "" {
				t.Fatalf("expected code cache write after captcha verification")
			}
		})
	}
}

func TestServiceSendCodeLoginRejectsAccountThatDoesNotMatchSelectedType(t *testing.T) {
	tests := []struct {
		name          string
		account       string
		loginType     string
		wantMessageID string
	}{
		{name: "phone entered on email tab", account: "15671628271", loginType: LoginTypeEmail, wantMessageID: "auth.send_code.email.invalid"},
		{name: "email entered on phone tab", account: "user@example.com", loginType: LoginTypePhone, wantMessageID: "auth.send_code.phone.invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCodeStore{}
			verifier := &fakeCaptchaVerifier{}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: []string{tt.loginType}},
				&fakeSessionCreator{},
				verifier,
				WithCodeStore(store),
				WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
				WithVerifyCodeMailSender(&fakeVerifyCodeMailSender{}),
			)

			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account:       tt.account,
				Scene:         VerifyCodeSceneLogin,
				LoginType:     tt.loginType,
				CaptchaID:     "captcha-id",
				CaptchaAnswer: &Answer{X: 120, Y: 80},
			})

			if message != "" || appErr == nil || appErr.MessageID != tt.wantMessageID {
				t.Fatalf("expected %s, message=%q err=%#v", tt.wantMessageID, message, appErr)
			}
			if verifier.input.ID != "" || store.setKey != "" {
				t.Fatalf("type mismatch must stop before consuming captcha or caching code: verifier=%#v store=%#v", verifier.input, store)
			}
		})
	}
}

func TestServicePhoneVerificationLoginCreatesNewUserWhenRegisterAllowed(t *testing.T) {
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	repo := &fakeAuthRepository{role: &DefaultRole{ID: 7}}
	sessions := &fakeSessionCreator{result: &TokenResult{
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
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute}),
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

func TestServiceCodeLoginRejectsUnavailableSelectedChannel(t *testing.T) {
	readiness := &fakeVerifyCodeReadinessProvider{readyByAccountType: map[string]bool{}}
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}, allowRegister: true},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(readiness),
	)
	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Code:         "123456",
		Platform:     "admin",
	})
	if result != nil || appErr == nil || appErr.Code != "auth.verify_code.channel_unavailable" {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
	if len(readiness.calls) != 1 || readiness.calls[0] != (verifyCodeReadinessCall{accountType: LoginTypePhone, scene: VerifyCodeSceneLogin}) {
		t.Fatalf("calls=%#v", readiness.calls)
	}
	if store.deleted != "" {
		t.Fatalf("unavailable channel consumed code: %q", store.deleted)
	}
}

func TestServiceCodeLoginPropagatesSelectedChannelReadinessFailure(t *testing.T) {
	wantErr := apperror.Internal("sms repository unavailable")
	readiness := &fakeVerifyCodeReadinessProvider{err: wantErr}
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}, allowRegister: true},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(readiness),
	)
	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Code:         "123456",
		Platform:     "admin",
	})
	if result != nil || appErr != wantErr {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
	wantCall := verifyCodeReadinessCall{accountType: LoginTypePhone, scene: VerifyCodeSceneLogin}
	if len(readiness.calls) != 1 || readiness.calls[0] != wantCall {
		t.Fatalf("calls=%#v", readiness.calls)
	}
	if store.getKey != "" || store.deleted != "" {
		t.Fatalf("store=%#v", store)
	}
}

func TestServiceCodeLoginChecksPlatformPolicyBeforeReadiness(t *testing.T) {
	readiness := &fakeVerifyCodeReadinessProvider{err: apperror.Internal("readiness must not be queried")}
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePassword}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(readiness),
	)
	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Code:         "123456",
		Platform:     "admin",
	})
	if result != nil || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "当前平台不支持该登录方式" {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
	if len(readiness.calls) != 0 || store.getKey != "" || store.deleted != "" {
		t.Fatalf("calls=%#v store=%#v", readiness.calls, store)
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
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute}),
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
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "暂未开放注册" {
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
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute}),
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
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
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

type fakeVerifyCodePhoneSender struct {
	scene string
	phone string
	code  string
	ttl   time.Duration
	err   *apperror.Error
}

func (f *fakeVerifyCodePhoneSender) SendVerifyCode(_ context.Context, scene, phone, code string, ttl time.Duration) *apperror.Error {
	f.scene = scene
	f.phone = phone
	f.code = code
	f.ttl = ttl
	return f.err
}

func (f *fakeVerifyCodeMailSender) SendVerifyCode(ctx context.Context, scene string, toEmail string, code string, ttl time.Duration) *apperror.Error {
	f.scene = scene
	f.email = toEmail
	f.code = code
	f.ttl = ttl
	return f.err
}

type fakeVerifyCodePolicyProvider struct {
	ttlByAccountType map[string]time.Duration
	ttl              time.Duration
	err              *apperror.Error
	accountTypes     []string
}

func (f *fakeVerifyCodePolicyProvider) VerifyCodeTTL(ctx context.Context, accountType string) (time.Duration, *apperror.Error) {
	f.accountTypes = append(f.accountTypes, accountType)
	if f.err != nil {
		return 0, f.err
	}
	if f.ttlByAccountType != nil {
		return f.ttlByAccountType[accountType], nil
	}
	return f.ttl, nil
}

func TestServiceSendCodeUsesPolicyTTLForEmailCacheAndMailSender(t *testing.T) {
	store := &fakeCodeStore{}
	mailSender := &fakeVerifyCodeMailSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodePolicyProvider(&fakeVerifyCodePolicyProvider{ttlByAccountType: map[string]time.Duration{LoginTypeEmail: 9 * time.Minute}}),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)
	_, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("user@example.com", LoginTypeEmail))
	if appErr != nil {
		t.Fatalf("unexpected err %#v", appErr)
	}
	if store.setTTL != 9*time.Minute || mailSender.ttl != 9*time.Minute {
		t.Fatalf("store=%s mail=%s", store.setTTL, mailSender.ttl)
	}
}

func TestServiceSendCodeUsesPolicyTTLForPhoneCache(t *testing.T) {
	store := &fakeCodeStore{}
	phoneSender := &fakeVerifyCodePhoneSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(phoneSender),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodePolicyProvider(&fakeVerifyCodePolicyProvider{ttlByAccountType: map[string]time.Duration{LoginTypePhone: 8 * time.Minute}}),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)
	_, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))
	if appErr != nil {
		t.Fatalf("unexpected err %#v", appErr)
	}
	if store.setCode != "654321" || store.setTTL != 8*time.Minute || phoneSender.ttl != store.setTTL {
		t.Fatalf("code=%q ttl=%s", store.setCode, store.setTTL)
	}
}

func TestServiceSendCodePassesAccountTypeToPolicyProvider(t *testing.T) {
	store := &fakeCodeStore{}
	policy := &fakeVerifyCodePolicyProvider{ttlByAccountType: map[string]time.Duration{LoginTypeEmail: 9 * time.Minute}}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeMailSender(&fakeVerifyCodeMailSender{}),
		WithVerifyCodePolicyProvider(policy),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	_, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("user@example.com", LoginTypeEmail))

	if appErr != nil {
		t.Fatalf("unexpected err %#v", appErr)
	}
	if len(policy.accountTypes) != 1 || policy.accountTypes[0] != LoginTypeEmail {
		t.Fatalf("policy account types = %#v", policy.accountTypes)
	}
}

func TestServiceSendCodeStopsWhenPolicyTTLInvalid(t *testing.T) {
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeMailSender(&fakeVerifyCodeMailSender{}),
		WithVerifyCodePolicyProvider(&fakeVerifyCodePolicyProvider{err: apperror.BadRequest("验证码有效期配置已禁用")}),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)
	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("user@example.com", LoginTypeEmail))
	if message != "" || appErr == nil || appErr.Message != "验证码有效期配置已禁用" {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.setKey != "" {
		t.Fatalf("must not write Redis before policy passes: %#v", store)
	}
}

func TestServiceSendCodeRealEmailUsesMailSender(t *testing.T) {
	store := &fakeCodeStore{}
	mailSender := &fakeVerifyCodeMailSender{}
	generatorCalls := 0
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) {
			generatorCalls++
			return "654321", nil
		}}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("user@example.com", LoginTypeEmail))

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
	if generatorCalls != 1 {
		t.Fatalf("generator calls=%d", generatorCalls)
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
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeMailSender(mailSender),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("user@example.com", LoginTypeEmail))

	if message != "" || appErr == nil || appErr.Message != "邮件发送失败" {
		t.Fatalf("expected mail failure, message=%q err=%#v", message, appErr)
	}
	if store.deleted != store.setKey || store.values[store.setKey] != "" {
		t.Fatalf("expected cached code cleanup, setKey=%q deleted=%q values=%#v", store.setKey, store.deleted, store.values)
	}
}

func TestServiceSendCodeDeletesPhoneVerificationWhenSMSFails(t *testing.T) {
	store := &fakeCodeStore{}
	wantErr := apperror.InternalKey("sms.send.failed", nil, "短信发送失败")
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodePhoneSender(&fakeVerifyCodePhoneSender{err: wantErr}),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))

	if message != "" || appErr != wantErr {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.deleted != store.setKey || store.values[store.setKey] != "" {
		t.Fatalf("store=%#v", store)
	}
}

func TestServiceSendCodeGenerationFailureDoesNotCacheOrSend(t *testing.T) {
	store := &fakeCodeStore{}
	phoneSender := &fakeVerifyCodePhoneSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(phoneSender),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{CodeGenerator: func() (string, error) { return "", errors.New("entropy unavailable") }}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))

	if message != "" || appErr == nil || appErr.Message != "验证码生成失败" {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.setKey != "" || phoneSender.phone != "" {
		t.Fatalf("store=%#v sender=%#v", store, phoneSender)
	}
}

func TestServiceSendCodePhoneRequiresSenderAndDeletesCachedCode(t *testing.T) {
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{LoginTypePhone}}, &fakeSessionCreator{}, &fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))

	if message != "" || appErr == nil || appErr.MessageID != "auth.verify_code.phone_unavailable" || appErr.Message != "短信验证码服务未配置" || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.deleted != store.setKey || store.values[store.setKey] != "" {
		t.Fatalf("store=%#v", store)
	}
}

func TestServiceSendCodeCacheFailureDoesNotSend(t *testing.T) {
	store := &fakeCodeStore{err: errors.New("cache unavailable")}
	phoneSender := &fakeVerifyCodePhoneSender{}
	service := NewService(
		&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{LoginTypePhone}}, &fakeSessionCreator{}, &fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(phoneSender),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone))

	if message != "" || appErr == nil || appErr.Message != "验证码缓存写入失败" {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if phoneSender.phone != "" {
		t.Fatalf("sender=%#v", phoneSender)
	}
}

func TestServiceSendCodeEmailRequiresMailSenderAndDeletesCachedCode(t *testing.T) {
	store := &fakeCodeStore{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodeReadinessProvider(allVerificationChannelsReady()),
		WithVerifyCodeOptions(VerifyCodeOptions{TTL: 5 * time.Minute, CodeGenerator: func() (string, error) { return "654321", nil }}),
	)

	message, appErr := service.SendCode(context.Background(), validLoginSendCodeInput("user@example.com", LoginTypeEmail))

	if message != "" || appErr == nil || appErr.MessageID != "auth.verify_code.email_unavailable" || appErr.Message != "邮件验证码服务未配置" || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("expected missing mail sender error, message=%q err=%#v", message, appErr)
	}
	if store.setCode != "654321" || store.deleted != store.setKey || store.values[store.setKey] != "" {
		t.Fatalf("email send-code must clean cached code when sender is missing, store=%#v", store)
	}
}

func TestRandomSixDigitCodeFromReaderPreservesLeadingZero(t *testing.T) {
	code, err := randomSixDigitCodeFromReader(bytes.NewReader(make([]byte, 3)))
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if code != "000000" {
		t.Fatalf("expected leading-zero code %q, got %q", "000000", code)
	}
	if len(code) != 6 {
		t.Fatalf("expected exactly six bytes, got %q", code)
	}
	for _, digit := range []byte(code) {
		if digit < '0' || digit > '9' {
			t.Fatalf("expected only ASCII digits, got %q", code)
		}
	}
}

func TestVerifyCodeCacheKeyUsesCodeOwnedNamespace(t *testing.T) {
	got := VerifyCodeCacheKey("email", VerifyCodeSceneLogin, "user@example.com")
	want := "auth:verify_code:email:login:b58996c504c5638798eb6b511e6f49af"
	if got != want {
		t.Fatalf("expected code-owned verify-code cache key %q, got %q", want, got)
	}
}
