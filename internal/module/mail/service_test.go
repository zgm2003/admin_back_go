package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"
)

type fakeMailRepository struct {
	config              *Config
	templates           map[string]*Template
	logs                map[uint64]Log
	logRows             map[uint64]LogReadRow
	listedLogRows       []LogReadRow
	created             Log
	createdVerification *VerificationCodeSnapshot
	nextLogID           uint64
	saved               *Config
	finish              LogFinish
	finishID            uint64
	createErr           error
	finishErr           error
	finishCalls         int
	finishCtx           context.Context
	finishCtxErr        error
	sequence            []string
	testAt              *time.Time
	testError           string
	err                 error
}

func (f *fakeMailRepository) DefaultConfig(ctx context.Context) (*Config, error) {
	return f.config, f.err
}

func (f *fakeMailRepository) SaveDefaultConfig(ctx context.Context, row Config) error {
	f.saved = &row
	return f.err
}

func (f *fakeMailRepository) SoftDeleteDefaultConfig(ctx context.Context) error { return f.err }

func (f *fakeMailRepository) UpdateConfigTestResult(ctx context.Context, at *time.Time, errorMessage string) error {
	f.testAt = at
	f.testError = errorMessage
	return f.err
}

func (f *fakeMailRepository) ListTemplates(ctx context.Context) ([]Template, error) {
	rows := make([]Template, 0, len(f.templates))
	for _, row := range f.templates {
		rows = append(rows, *row)
	}
	return rows, f.err
}

func (f *fakeMailRepository) TemplateByID(ctx context.Context, id uint64) (*Template, error) {
	for _, row := range f.templates {
		if row.ID == id {
			return row, f.err
		}
	}
	return nil, f.err
}

func (f *fakeMailRepository) TemplateByScene(ctx context.Context, scene string) (*Template, error) {
	return f.templates[scene], f.err
}

func (f *fakeMailRepository) SaveTemplate(ctx context.Context, row Template) (uint64, error) {
	return 1, f.err
}
func (f *fakeMailRepository) UpdateTemplate(ctx context.Context, id uint64, update TemplateUpdate) error {
	return f.err
}
func (f *fakeMailRepository) SoftDeleteTemplate(ctx context.Context, id uint64) error { return f.err }

func (f *fakeMailRepository) CreateLog(ctx context.Context, row Log) (uint64, error) {
	f.sequence = append(f.sequence, "create-log")
	if f.createErr != nil {
		return 0, f.createErr
	}
	if f.logs == nil {
		f.logs = map[uint64]Log{}
	}
	f.nextLogID++
	row.ID = f.nextLogID
	f.logs[row.ID] = row
	f.created = row
	return row.ID, f.err
}

func (f *fakeMailRepository) CreateVerificationLog(ctx context.Context, row Log, snapshot VerificationCodeSnapshot) (uint64, error) {
	f.sequence = append(f.sequence, "create-verification-log")
	if f.createErr != nil {
		return 0, f.createErr
	}
	if f.logs == nil {
		f.logs = map[uint64]Log{}
	}
	f.nextLogID++
	row.ID = f.nextLogID
	f.logs[row.ID] = row
	f.created = row
	id := row.ID
	snapshot.MailLogID = id
	f.createdVerification = &snapshot
	return id, nil
}

func (f *fakeMailRepository) FinishLog(ctx context.Context, id uint64, finish LogFinish) error {
	f.sequence = append(f.sequence, "finish-log")
	f.finishCalls++
	f.finishCtx = ctx
	f.finishCtxErr = ctx.Err()
	f.finishID = id
	f.finish = finish
	row := f.logs[id]
	row.Status = finish.Status
	row.TencentRequestID = finish.RequestID
	row.TencentMessageID = finish.MessageID
	row.ErrorCode = finish.ErrorCode
	row.ErrorMessage = finish.ErrorMessage
	row.DurationMS = finish.DurationMS
	row.SentAt = finish.SentAt
	f.logs[id] = row
	if f.finishErr != nil {
		return f.finishErr
	}
	return f.err
}

func (f *fakeMailRepository) ListLogRows(ctx context.Context, query LogQuery) ([]LogReadRow, int64, error) {
	if f.listedLogRows != nil {
		return append([]LogReadRow(nil), f.listedLogRows...), int64(len(f.listedLogRows)), f.err
	}
	if f.logRows != nil {
		rows := make([]LogReadRow, 0, len(f.logRows))
		for _, row := range f.logRows {
			rows = append(rows, row)
		}
		return rows, int64(len(rows)), f.err
	}
	rows := make([]LogReadRow, 0, len(f.logs))
	for _, row := range f.logs {
		rows = append(rows, LogReadRow{Log: row})
	}
	return rows, int64(len(rows)), f.err
}

func (f *fakeMailRepository) LogRowByID(ctx context.Context, id uint64) (*LogReadRow, error) {
	if row, ok := f.logRows[id]; ok {
		return &row, f.err
	}
	row, ok := f.logs[id]
	if !ok {
		return nil, f.err
	}
	return &LogReadRow{Log: row}, f.err
}

func (f *fakeMailRepository) SoftDeleteLogs(ctx context.Context, ids []uint64) error { return f.err }

type fakeMailSender struct {
	input    SendInput
	result   SendResult
	err      error
	ctx      context.Context
	calls    int
	sequence *[]string
}

func (f *fakeMailSender) Send(ctx context.Context, input SendInput) (SendResult, error) {
	f.ctx = ctx
	f.calls++
	if f.sequence != nil {
		*f.sequence = append(*f.sequence, "sender")
	}
	f.input = input
	if f.err != nil {
		return SendResult{}, f.err
	}
	return f.result, nil
}

func testDiagnosticBox() secretbox.VersionedBox {
	box, err := secretbox.NewVersioned("diag-current", map[string][]byte{
		"diag-current":  []byte("12345678901234567890123456789012"),
		"diag-previous": []byte("abcdefghijklmnopqrstuvwxyzABCDEF"),
	})
	if err != nil {
		panic(err)
	}
	return box
}

func diagnosticService(repo Repository, credentialBox secretbox.Box, diagnosticBox secretbox.VersionedBox, sender Sender, now time.Time) *Service {
	return NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: credentialBox, DiagnosticBox: diagnosticBox,
		Sender: sender, Clock: clock.Func(func() time.Time { return now }),
	})
}

func configuredVerifyRepository(box secretbox.Box, scene string) *fakeMailRepository {
	secretIDEnc, _ := box.Encrypt("AKID-secret")
	secretKeyEnc, _ := box.Encrypt("SECRET-key")
	return &fakeMailRepository{
		config: &Config{SecretIDEnc: secretIDEnc, SecretKeyEnc: secretKeyEnc, Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes},
		templates: map[string]*Template{
			scene: {ID: 77, Scene: scene, Subject: "Login code", TencentTemplateID: 123456, VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes},
		},
	}
}

func TestSendVerifyCodeEncryptsAndCommitsBeforeProviderIO(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	sender := &fakeMailSender{result: SendResult{RequestID: "req", MessageID: "msg"}, sequence: &repo.sequence}
	service := diagnosticService(repo, credentialBox, testDiagnosticBox(), sender, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))

	if err != nil {
		t.Fatalf("SendVerifyCode returned error: %v", err)
	}
	if got := strings.Join(repo.sequence, ","); got != "create-verification-log,sender,finish-log" {
		t.Fatalf("expected commit before provider and finish after provider, got %q", got)
	}
	if repo.createdVerification == nil || repo.createdVerification.KeyID != "diag-current" || repo.createdVerification.CodeEnc == "" {
		t.Fatalf("expected encrypted verification snapshot, got %#v", repo.createdVerification)
	}
}

func TestSendVerifyCodeDoesNotCreateVerificationSnapshotWithoutDiagnosticBox(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	sender := &fakeMailSender{}
	service := diagnosticService(repo, credentialBox, secretbox.VersionedBox{}, sender, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))

	if err == nil || err.Message != "邮件验证码诊断加密未配置" {
		t.Fatalf("expected safe diagnostic configuration error, got %#v", err)
	}
	if repo.created.ID != 0 || repo.createdVerification != nil || sender.calls != 0 {
		t.Fatalf("diagnostic setup failure must precede writes and provider I/O: repo=%#v child=%#v calls=%d", repo.created, repo.createdVerification, sender.calls)
	}
}

func TestServiceDependenciesDoesNotExposeDiagnosticEncrypt(t *testing.T) {
	if field, ok := reflect.TypeOf(ServiceDependencies{}).FieldByName("DiagnosticEncrypt"); ok {
		t.Fatalf("ServiceDependencies must not expose diagnostic encryption override %q", field.Name)
	}
}

func TestSendVerifyCodeMapsDiagnosticEncryptionFailureToInvalidSnapshot(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	sender := &fakeMailSender{}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	service := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: credentialBox, DiagnosticBox: testDiagnosticBox(), Sender: sender,
		Clock: clock.Func(func() time.Time { return now }),
	})
	service.diagnosticEncrypt = func(string) (string, string, error) {
		return "", "", errors.New("encrypt contains SECRET-key and 654321")
	}

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, now.Add(5*time.Minute))

	if err == nil || err.Message != "邮件验证码诊断加密失败" || !errors.Is(err, ErrInvalidDiagnosticSnapshot) {
		t.Fatalf("expected invalid diagnostic snapshot error, got %#v", err)
	}
	for _, sensitive := range []string{"encrypt contains", "SECRET-key", "654321"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("diagnostic encryption failure leaked %q: %v", sensitive, err)
		}
	}
	if repo.created.ID != 0 || repo.createdVerification != nil || sender.calls != 0 {
		t.Fatalf("encryption failure must precede writes and provider I/O: repo=%#v child=%#v calls=%d", repo.created, repo.createdVerification, sender.calls)
	}
}

func TestSendVerifyCodeValidatesASCIICodeTTLAndDeadline(t *testing.T) {
	credentialBox := testSecretBox()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	service := diagnosticService(&fakeMailRepository{}, credentialBox, testDiagnosticBox(), &fakeMailSender{}, now)
	tests := []struct {
		name string
		code string
		ttl  time.Duration
		exp  time.Time
	}{
		{name: "non ascii digits", code: "１２３４５６", ttl: 5 * time.Minute, exp: now.Add(5 * time.Minute)},
		{name: "wrong length", code: "12345", ttl: 5 * time.Minute, exp: now.Add(5 * time.Minute)},
		{name: "surrounding whitespace", code: " 123456 ", ttl: 5 * time.Minute, exp: now.Add(5 * time.Minute)},
		{name: "zero ttl", code: "123456", ttl: 0, exp: now.Add(5 * time.Minute)},
		{name: "past expiry", code: "123456", ttl: 5 * time.Minute, exp: now.Add(-time.Second)},
		{name: "subsecond expiry", code: "123456", ttl: 5 * time.Minute, exp: now.Add(5*time.Minute + time.Nanosecond)},
		{name: "expiry exceeds ttl", code: "123456", ttl: 5 * time.Minute, exp: now.Add(6 * time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", tt.code, tt.ttl, tt.exp)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if tt.name == "surrounding whitespace" && err.Message != "验证码必须是六位数字" {
				t.Fatalf("surrounding whitespace must fail strict code validation, got %#v", err)
			}
		})
	}
}

func TestSendVerifyCodeTransactionFailureSkipsProvider(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	repo.createErr = errors.New("transaction contains 654321")
	sender := &fakeMailSender{}
	service := diagnosticService(repo, credentialBox, testDiagnosticBox(), sender, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))

	if err == nil || err.Message != "写入邮件验证码诊断失败" || err.Cause != nil {
		t.Fatalf("expected fixed transaction error without cause, got %#v", err)
	}
	if sender.calls != 0 || repo.createdVerification != nil {
		t.Fatalf("transaction failure must skip provider: calls=%d child=%#v", sender.calls, repo.createdVerification)
	}
}

type deadlineBindingContext struct {
	context.Context
	bound *bool
}

func (c deadlineBindingContext) Deadline() (time.Time, bool) {
	*c.bound = true
	return time.Time{}, false
}

func TestSendVerifyCodeSkipsProviderWhenFinalPreSendClockReadExpires(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(5 * time.Minute)
	providerContextBound := false
	sender := &fakeMailSender{}
	service := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: credentialBox, DiagnosticBox: testDiagnosticBox(), Sender: sender,
		Clock: clock.Func(func() time.Time {
			if providerContextBound {
				return expiresAt
			}
			return now
		}),
	})
	ctx := deadlineBindingContext{Context: context.Background(), bound: &providerContextBound}

	err := service.SendVerifyCode(ctx, enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, expiresAt)

	if err == nil || err.Message != "验证码发送已过期" || !errors.Is(err, ErrVerificationDeadlineElapsed) {
		t.Fatalf("expected safe deadline error, got %#v", err)
	}
	if sender.calls != 0 || repo.finish.Status != enum.MailLogStatusFailed || repo.finish.ErrorCode != "verification_deadline_elapsed" {
		t.Fatalf("deadline must finalize parent without provider: calls=%d finish=%#v", sender.calls, repo.finish)
	}
	if child := repo.createdVerification; child == nil || child.MailLogID != 1 || child.KeyID != "diag-current" || child.CodeEnc == "" || !child.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("deadline must finalize parent without provider and preserve child: calls=%d finish=%#v child=%#v", sender.calls, repo.finish, repo.createdVerification)
	}
}

func TestSendVerifyCodeDeadlineFailureRemainsPrimaryWhenFinalizationFails(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	repo.finishErr = errors.New("finish contains SECRET-key and 654321")
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(5 * time.Minute)
	reads := 0
	sender := &fakeMailSender{}
	service := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: credentialBox, DiagnosticBox: testDiagnosticBox(), Sender: sender,
		Clock: clock.Func(func() time.Time {
			reads++
			if reads > 1 {
				return expiresAt
			}
			return now
		}),
	})

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, expiresAt)

	if err == nil || err.Message != "验证码发送已过期" || !errors.Is(err, ErrVerificationDeadlineElapsed) {
		t.Fatalf("deadline error must remain primary, got %#v", err)
	}
	for _, sensitive := range []string{"finish contains", "SECRET-key", "654321"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("finalization failure leaked %q: %v", sensitive, err)
		}
	}
	if sender.calls != 0 || repo.finishCalls != 1 || repo.finish.Status != enum.MailLogStatusFailed || repo.finish.ErrorCode != verificationDeadlineErrorCode {
		t.Fatalf("deadline branch must skip provider and attempt fixed parent finalization: calls=%d finishCalls=%d finish=%#v", sender.calls, repo.finishCalls, repo.finish)
	}
	if child := repo.createdVerification; child == nil || child.MailLogID != 1 || child.KeyID != "diag-current" || child.CodeEnc == "" || !child.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("failed parent finalization must leave child unchanged: %#v", child)
	}
}

func TestSendVerifyCodeBindsProviderContextToEarlierDeadline(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	sender := &fakeMailSender{result: SendResult{RequestID: "req", MessageID: "msg"}}
	service := diagnosticService(repo, credentialBox, testDiagnosticBox(), sender, now)
	incomingDeadline := now.Add(2 * time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), incomingDeadline)
	defer cancel()

	err := service.SendVerifyCode(ctx, enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, now.Add(5*time.Minute))

	if err != nil {
		t.Fatalf("SendVerifyCode returned error: %v", err)
	}
	deadline, ok := sender.ctx.Deadline()
	if !ok || !deadline.Equal(incomingDeadline) {
		t.Fatalf("provider context must retain earlier incoming deadline, got %v", deadline)
	}
}

func TestSendVerifyCodeProviderFailureRemainsPrimaryWhenFinalizationFails(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	repo.finishErr = errors.New("database contains 654321")
	sender := &fakeMailSender{err: codedMailTestError{code: "FailedOperation.TemplateNotApproved", msg: "provider contains SECRET-key and 654321"}}
	service := diagnosticService(repo, credentialBox, testDiagnosticBox(), sender, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))

	if err == nil || err.Message != "邮件发送失败" || strings.Contains(err.Error(), "SECRET-key") || strings.Contains(err.Error(), "654321") {
		t.Fatalf("provider failure must remain primary and safe, got %#v", err)
	}
	if repo.createdVerification == nil || repo.finish.ErrorMessage != "邮件服务调用失败" {
		t.Fatalf("failed finalization must not mutate child or persist raw provider error: child=%#v finish=%#v", repo.createdVerification, repo.finish)
	}
}

func TestSendVerifyCodeFinalizesAfterCallerCancellation(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := SenderFunc(func(context.Context, SendInput) (SendResult, error) {
		cancel()
		return SendResult{}, errors.New("provider contains 654321")
	})
	service := diagnosticService(repo, credentialBox, testDiagnosticBox(), sender, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))

	err := service.SendVerifyCode(ctx, enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))

	if err == nil || err.Message != "邮件发送失败" {
		t.Fatalf("provider failure must remain primary, got %#v", err)
	}
	if repo.finishCalls != 1 || repo.finish.Status != enum.MailLogStatusFailed || repo.finishCtx == nil || repo.finishCtxErr != nil {
		t.Fatalf("finalization must ignore caller cancellation: calls=%d finish=%#v ctx=%v", repo.finishCalls, repo.finish, repo.finishCtx)
	}
}

func TestSendVerifyCodeDropsRawConfigurationAndTemplateErrors(t *testing.T) {
	credentialBox := testSecretBox()
	raw := errors.New("database contains AKID-secret SECRET-key 654321 ciphertext")
	tests := []struct {
		name string
		repo *verificationErrorRepository
		want string
	}{
		{name: "configuration", repo: &verificationErrorRepository{fakeMailRepository: configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin), configErr: raw}, want: "查询邮件配置失败"},
		{name: "template", repo: &verificationErrorRepository{fakeMailRepository: configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin), templateErr: raw}, want: "查询邮件模板失败"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := diagnosticService(tt.repo, credentialBox, testDiagnosticBox(), &fakeMailSender{}, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))
			err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))
			if err == nil || err.Message != tt.want || err.Cause != nil {
				t.Fatalf("expected fixed error without cause, got %#v", err)
			}
			for _, sensitive := range []string{"AKID-secret", "SECRET-key", "654321", "ciphertext"} {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("raw dependency failure leaked %q: %v", sensitive, err)
				}
			}
		})
	}
}

type verificationErrorRepository struct {
	*fakeMailRepository
	configErr   error
	templateErr error
}

func (r *verificationErrorRepository) DefaultConfig(ctx context.Context) (*Config, error) {
	if r.configErr != nil {
		return nil, r.configErr
	}
	return r.fakeMailRepository.DefaultConfig(ctx)
}

func (r *verificationErrorRepository) TemplateByScene(ctx context.Context, scene string) (*Template, error) {
	if r.templateErr != nil {
		return nil, r.templateErr
	}
	return r.fakeMailRepository.TemplateByScene(ctx, scene)
}

func TestSendVerifyCodeSuccessFinalizationFailureIsFixedAndChildUnchanged(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	repo.finishErr = errors.New("finish contains 654321")
	sender := &fakeMailSender{result: SendResult{RequestID: "req", MessageID: "msg"}}
	service := diagnosticService(repo, credentialBox, testDiagnosticBox(), sender, time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC))

	if err == nil || err.Message != "更新邮件日志失败" || strings.Contains(err.Error(), "654321") {
		t.Fatalf("expected fixed finalization error, got %#v", err)
	}
	if repo.createdVerification == nil || repo.createdVerification.CodeEnc == "" {
		t.Fatalf("finalization must not mutate child: %#v", repo.createdVerification)
	}
}

type codedMailTestError struct {
	code string
	msg  string
}

func (e codedMailTestError) Error() string     { return e.msg }
func (e codedMailTestError) ErrorCode() string { return e.code }

func TestServiceSendVerifyCodeUsesEnabledConfigTemplateAndWritesSanitizedLogs(t *testing.T) {
	box := testSecretBox()
	secretIDEnc, err := box.Encrypt("AKID-secret")
	if err != nil {
		t.Fatalf("encrypt secret id: %v", err)
	}
	secretKeyEnc, err := box.Encrypt("SECRET-key")
	if err != nil {
		t.Fatalf("encrypt secret key: %v", err)
	}
	repo := &fakeMailRepository{
		config: &Config{SecretIDEnc: secretIDEnc, SecretKeyEnc: secretKeyEnc, Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", FromName: "Admin", Status: enum.CommonYes},
		templates: map[string]*Template{
			enum.VerifyCodeSceneLogin: {ID: 77, Scene: enum.VerifyCodeSceneLogin, Subject: "Login code", TencentTemplateID: 123456, VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes},
		},
	}
	sender := &fakeMailSender{result: SendResult{RequestID: "req-1", MessageID: "msg-1"}}
	now := time.Now().Truncate(time.Second)
	service := diagnosticService(repo, box, testDiagnosticBox(), sender, now)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, now.Add(5*time.Minute))

	if appErr != nil {
		t.Fatalf("expected SendVerifyCode to succeed, got %v", appErr)
	}
	if sender.input.SecretID != "AKID-secret" || sender.input.SecretKey != "SECRET-key" {
		t.Fatalf("sender must receive decrypted credentials, got %#v", sender.input)
	}
	if sender.input.TemplateID != 123456 || sender.input.TemplateData["code"] != "654321" || sender.input.TemplateData["ttl_minutes"] != "5" {
		t.Fatalf("unexpected sender template payload: %#v", sender.input)
	}
	forbiddenKey := "app" + "_name"
	if _, ok := sender.input.TemplateData[forbiddenKey]; ok {
		t.Fatalf("verify-code TemplateData must not include %s: %#v", forbiddenKey, sender.input.TemplateData)
	}
	if len(sender.input.TemplateData) != 2 {
		t.Fatalf("verify-code TemplateData must contain exactly code and ttl_minutes, got %#v", sender.input.TemplateData)
	}
	created := repo.created
	if created.Scene != enum.VerifyCodeSceneLogin || created.TemplateID == nil || *created.TemplateID != 77 || created.Status != enum.MailLogStatusPending {
		t.Fatalf("unexpected pending log: %#v", created)
	}
	if strings.Contains(created.ErrorMessage, "654321") || strings.Contains(created.Subject, "654321") {
		t.Fatalf("mail log must not persist verify code: %#v", created)
	}
	if repo.finishID != 1 || repo.finish.Status != enum.MailLogStatusSuccess || repo.finish.RequestID != "req-1" || repo.finish.MessageID != "msg-1" {
		t.Fatalf("unexpected finish log: id=%d finish=%#v", repo.finishID, repo.finish)
	}
}

func TestServiceSendVerifyCodeFailureStoresProviderErrorCodeOnly(t *testing.T) {
	box := testSecretBox()
	secretIDEnc, _ := box.Encrypt("AKID-secret")
	secretKeyEnc, _ := box.Encrypt("SECRET-key")
	repo := &fakeMailRepository{
		config: &Config{SecretIDEnc: secretIDEnc, SecretKeyEnc: secretKeyEnc, Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes},
		templates: map[string]*Template{
			enum.VerifyCodeSceneForget: {ID: 78, Scene: enum.VerifyCodeSceneForget, Subject: "Reset code", TencentTemplateID: 123457, VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes},
		},
	}
	sender := &fakeMailSender{err: codedMailTestError{code: "FailedOperation.TemplateNotApproved", msg: "template not approved"}}
	now := time.Now().Truncate(time.Second)
	service := diagnosticService(repo, box, testDiagnosticBox(), sender, now)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneForget, "user@example.com", "654321", 5*time.Minute, now.Add(5*time.Minute))

	if appErr == nil || appErr.Message != "邮件发送失败" {
		t.Fatalf("expected send failure, got %#v", appErr)
	}
	if repo.finish.Status != enum.MailLogStatusFailed || repo.finish.ErrorCode != "FailedOperation.TemplateNotApproved" || repo.finish.ErrorMessage != "邮件服务调用失败" {
		t.Fatalf("unexpected failed finish log: %#v", repo.finish)
	}
	if strings.Contains(repo.finish.ErrorMessage, "654321") || strings.Contains(repo.finish.ErrorMessage, "TemplateData") {
		t.Fatalf("failed log must not persist verify code or template data: %#v", repo.finish)
	}
}

func TestTestSendDoesNotCreateVerificationSnapshot(t *testing.T) {
	box := testSecretBox()
	secretIDEnc, _ := box.Encrypt("AKID-secret")
	secretKeyEnc, _ := box.Encrypt("SECRET-key")
	repo := &fakeMailRepository{
		config: &Config{SecretIDEnc: secretIDEnc, SecretKeyEnc: secretKeyEnc, Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes},
		templates: map[string]*Template{
			enum.VerifyCodeSceneLogin: {ID: 77, Scene: enum.VerifyCodeSceneLogin, Subject: "Login code", TencentTemplateID: 123456, VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes},
		},
	}
	service := NewService(repo, box, &fakeMailSender{result: SendResult{RequestID: "req", MessageID: "msg"}})

	appErr := service.TestSend(context.Background(), TestInput{ToEmail: "user@example.com", TemplateScene: enum.VerifyCodeSceneLogin})

	if appErr != nil {
		t.Fatalf("TestSend returned error: %v", appErr)
	}
	if repo.created.ID == 0 || repo.created.Scene != enum.MailSceneTest {
		t.Fatalf("TestSend must create a parent log: %#v", repo.created)
	}
	if repo.createdVerification != nil {
		t.Fatalf("TestSend must not create a verification child: %#v", repo.createdVerification)
	}
}

func TestServiceConfigResponseDoesNotExposeEncryptedSecrets(t *testing.T) {
	repo := &fakeMailRepository{
		config: &Config{ID: 1, SecretIDEnc: "cipher-id", SecretIDHint: "***t-id", SecretKeyEnc: "cipher-key", SecretKeyHint: "***-key", Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: 6},
	}
	service := NewService(repo, testSecretBox(), &fakeMailSender{})

	result, appErr := service.Config(context.Background())
	if appErr != nil {
		t.Fatalf("expected Config to succeed, got %v", appErr)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal config response: %v", err)
	}
	jsonText := string(body)
	if strings.Contains(jsonText, "secret_id_enc") || strings.Contains(jsonText, "secret_key_enc") || strings.Contains(jsonText, "cipher-id") || strings.Contains(jsonText, "cipher-key") {
		t.Fatalf("config response leaked encrypted secrets: %s", jsonText)
	}
	if result.SecretIDHint != "***t-id" || result.SecretKeyHint != "***-key" {
		t.Fatalf("config response must return hints, got %#v", result)
	}
}

func TestServiceConfigIncludesVerifyCodeTTLFromConfigRow(t *testing.T) {
	repo := &fakeMailRepository{
		config: &Config{ID: 1, SecretIDHint: "***t-id", SecretKeyHint: "***-key", Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: 11},
	}
	service := NewService(repo, testSecretBox(), &fakeMailSender{})

	result, appErr := service.Config(context.Background())

	if appErr != nil {
		t.Fatalf("expected Config to succeed, got %v", appErr)
	}
	if result.VerifyCodeTTLMinutes != 11 {
		t.Fatalf("expected ttl 11, got %#v", result)
	}
}

func TestServiceDefaultConfigUsesDefaultVerifyCodeTTLWhenConfigMissing(t *testing.T) {
	service := NewService(&fakeMailRepository{}, testSecretBox(), &fakeMailSender{})

	result, appErr := service.Config(context.Background())

	if appErr != nil {
		t.Fatalf("expected Config to succeed, got %v", appErr)
	}
	if result.Configured || result.VerifyCodeTTLMinutes != 5 {
		t.Fatalf("expected unconfigured config with default ttl 5, got %#v", result)
	}
}

func TestServicePageInitExposesTencentSESRegions(t *testing.T) {
	service := NewService(&fakeMailRepository{}, testSecretBox(), &fakeMailSender{})

	result, appErr := service.PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("expected PageInit to succeed, got %v", appErr)
	}
	if result.Dict.DefaultRegion != DefaultRegion {
		t.Fatalf("unexpected default region: %#v", result.Dict)
	}
	if len(result.Dict.MailRegionArr) != 2 {
		t.Fatalf("expected exactly Tencent SES SendEmail supported regions, got %#v", result.Dict.MailRegionArr)
	}
	if result.Dict.MailRegionArr[0].Value != "ap-guangzhou" || result.Dict.MailRegionArr[1].Value != "ap-hongkong" {
		t.Fatalf("unexpected region options: %#v", result.Dict.MailRegionArr)
	}
}

func TestServiceSaveConfigRejectsUnsupportedRegion(t *testing.T) {
	box := testSecretBox()
	service := NewService(&fakeMailRepository{}, box, &fakeMailSender{})

	appErr := service.SaveConfig(context.Background(), SaveConfigInput{
		SecretID: "AKID-secret", SecretKey: "SECRET-key", Region: "ap-shanghai", Endpoint: DefaultEndpoint,
		FromEmail: "noreply@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: 5,
	})
	if appErr == nil || !strings.Contains(appErr.Message, "不支持的腾讯云 SES 地域") {
		t.Fatalf("expected unsupported region error, got %#v", appErr)
	}
}

func TestServiceLogDetailIncludesTemplateSummaryWithoutPayload(t *testing.T) {
	templateID := uint64(79)
	repo := &fakeMailRepository{
		templates: map[string]*Template{
			enum.VerifyCodeSceneLogin: {
				ID: templateID, Scene: enum.VerifyCodeSceneLogin, Name: "验证码登录", Subject: "Login",
				TencentTemplateID: 31463, VariablesJSON: `["code","ttl_minutes"]`,
				SampleVariablesJSON: `{"code":"654321","ttl_minutes":"5"}`, Status: enum.CommonYes,
			},
		},
		logs: map[uint64]Log{
			7: {ID: 7, Scene: enum.MailSceneTest, TemplateID: &templateID, ToEmail: "user@example.com", Subject: "Login", Status: enum.MailLogStatusSuccess},
		},
	}
	service := NewService(repo, testSecretBox(), &fakeMailSender{})

	result, appErr := service.Log(context.Background(), 7)
	if appErr != nil {
		t.Fatalf("expected Log to succeed, got %v", appErr)
	}
	if result.Template == nil || result.Template.TencentTemplateID != 31463 || len(result.Template.Variables) != 2 {
		t.Fatalf("expected template summary in log detail, got %#v", result.Template)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal log detail: %v", err)
	}
	jsonText := string(body)
	if strings.Contains(jsonText, "654321") || strings.Contains(jsonText, "sample_variables") || strings.Contains(jsonText, "template_data") {
		t.Fatalf("log detail leaked template payload: %s", jsonText)
	}
}

func TestServiceSaveConfigRequiresSecretsOnFirstConfigAndReusesExistingSecretsOnEdit(t *testing.T) {
	box := testSecretBox()
	service := NewService(&fakeMailRepository{}, box, &fakeMailSender{})

	appErr := service.SaveConfig(context.Background(), SaveConfigInput{Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: 5})
	if appErr == nil || !strings.Contains(appErr.Message, "首次配置必须填写") {
		t.Fatalf("expected first config secret error, got %#v", appErr)
	}

	existingID, _ := box.Encrypt("AKID-existing")
	existingKey, _ := box.Encrypt("SECRET-existing")
	repo := &fakeMailRepository{config: &Config{SecretIDEnc: existingID, SecretIDHint: "***ting", SecretKeyEnc: existingKey, SecretKeyHint: "***ting", Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "old@example.com", Status: enum.CommonYes}}
	service = NewService(repo, box, &fakeMailSender{})

	appErr = service.SaveConfig(context.Background(), SaveConfigInput{Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "new@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: 5})
	if appErr != nil {
		t.Fatalf("expected edit to reuse existing secrets, got %v", appErr)
	}
	if repo.saved == nil || repo.saved.SecretIDEnc != existingID || repo.saved.SecretKeyEnc != existingKey || repo.saved.FromEmail != "new@example.com" {
		t.Fatalf("unexpected saved config: %#v", repo.saved)
	}
}

func TestServiceSaveConfigPersistsVerifyCodeTTLToConfigRow(t *testing.T) {
	box := testSecretBox()
	secretIDEnc, _ := box.Encrypt("AKID-existing")
	secretKeyEnc, _ := box.Encrypt("SECRET-existing")
	repo := &fakeMailRepository{config: &Config{SecretIDEnc: secretIDEnc, SecretIDHint: "***ting", SecretKeyEnc: secretKeyEnc, SecretKeyHint: "***ting", Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "old@example.com", Status: enum.CommonYes}}
	service := NewService(repo, box, &fakeMailSender{})

	appErr := service.SaveConfig(context.Background(), SaveConfigInput{Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "new@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: 11})

	if appErr != nil {
		t.Fatalf("expected SaveConfig to succeed, got %v", appErr)
	}
	if repo.saved == nil || repo.saved.VerifyCodeTTLMinutes != 11 {
		t.Fatalf("unexpected saved config ttl: %#v", repo.saved)
	}
}

func TestServiceVerifyCodeTTLUsesConfigRow(t *testing.T) {
	service := NewService(&fakeMailRepository{config: &Config{VerifyCodeTTLMinutes: 13}}, testSecretBox(), &fakeMailSender{})

	got, appErr := service.VerifyCodeTTL(context.Background())

	if appErr != nil || got != 13*time.Minute {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
}

func TestServiceVerifyCodeTTLRejectsMissingConfig(t *testing.T) {
	service := NewService(&fakeMailRepository{}, testSecretBox(), &fakeMailSender{})

	got, appErr := service.VerifyCodeTTL(context.Background())

	if appErr == nil || appErr.Message != "邮件验证码配置未启用" || got != 0 {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
}

func TestServiceVerifyCodeTTLRejectsInvalidConfigRow(t *testing.T) {
	for _, ttl := range []int{0, 61} {
		service := NewService(&fakeMailRepository{config: &Config{VerifyCodeTTLMinutes: ttl}}, testSecretBox(), &fakeMailSender{})
		got, appErr := service.VerifyCodeTTL(context.Background())
		if appErr == nil || appErr.Message != "验证码有效期必须在 1-60 分钟之间" || got != 0 {
			t.Fatalf("ttl=%d got duration=%s err=%#v", ttl, got, appErr)
		}
	}
}

func TestServiceSaveConfigRejectsInvalidVerifyCodeTTL(t *testing.T) {
	box := testSecretBox()
	tests := []struct {
		name string
		ttl  int
	}{
		{name: "zero", ttl: 0},
		{name: "too large", ttl: 61},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&fakeMailRepository{}, box, &fakeMailSender{})
			appErr := service.SaveConfig(context.Background(), SaveConfigInput{
				SecretID: "AKID-secret", SecretKey: "SECRET-key", Region: DefaultRegion, Endpoint: DefaultEndpoint,
				FromEmail: "noreply@example.com", Status: enum.CommonYes, VerifyCodeTTLMinutes: tt.ttl,
			})
			if appErr == nil || appErr.Message != "验证码有效期必须在 1-60 分钟之间" {
				t.Fatalf("ttl=%d got %#v", tt.ttl, appErr)
			}
		})
	}
}

func TestServiceRejectsVerifyCodeTemplateWithAppNameVariable(t *testing.T) {
	service := NewService(&fakeMailRepository{}, testSecretBox(), &fakeMailSender{})
	_, appErr := service.CreateTemplate(context.Background(), SaveTemplateInput{
		Scene:             enum.VerifyCodeSceneLogin,
		Name:              "登录验证码",
		Subject:           "Login",
		TencentTemplateID: 123456,
		Variables:         []string{"app_name", "code", "ttl_minutes"},
		SampleVariables:   map[string]string{"app_name": "admin_go", "code": "123456", "ttl_minutes": "5"},
		Status:            enum.CommonYes,
	})
	if appErr == nil || appErr.Message != "验证码模板变量必须且只能是 code、ttl_minutes" {
		t.Fatalf("got %#v", appErr)
	}
}

func TestServiceRejectsVerifyCodeTemplateWithExtraSampleVariable(t *testing.T) {
	service := NewService(&fakeMailRepository{}, testSecretBox(), &fakeMailSender{})
	_, appErr := service.CreateTemplate(context.Background(), SaveTemplateInput{
		Scene:             enum.VerifyCodeSceneForget,
		Name:              "找回密码验证码",
		Subject:           "Reset",
		TencentTemplateID: 123457,
		Variables:         []string{"code", "ttl_minutes"},
		SampleVariables:   map[string]string{"code": "123456", "ttl_minutes": "5", "app_name": "admin_go"},
		Status:            enum.CommonYes,
	})
	if appErr == nil || appErr.Message != "验证码模板测试变量必须且只能是 code、ttl_minutes" {
		t.Fatalf("got %#v", appErr)
	}
}

func TestServiceRejectsMissingTemplateVariablesBeforeSending(t *testing.T) {
	box := testSecretBox()
	secretIDEnc, _ := box.Encrypt("AKID-secret")
	secretKeyEnc, _ := box.Encrypt("SECRET-key")
	repo := &fakeMailRepository{
		config: &Config{SecretIDEnc: secretIDEnc, SecretKeyEnc: secretKeyEnc, Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes},
		templates: map[string]*Template{
			enum.VerifyCodeSceneBindEmail: {ID: 79, Scene: enum.VerifyCodeSceneBindEmail, Subject: "Bind", TencentTemplateID: 123458, VariablesJSON: `["code","ttl_minutes","missing"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5","missing":"x"}`, Status: enum.CommonYes},
		},
	}
	sender := &fakeMailSender{}
	now := time.Now().Truncate(time.Second)
	service := diagnosticService(repo, box, testDiagnosticBox(), sender, now)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneBindEmail, "user@example.com", "654321", 5*time.Minute, now.Add(5*time.Minute))
	if appErr == nil || !strings.Contains(appErr.Message, "邮件模板变量缺少 missing") {
		t.Fatalf("expected missing variable error, got %#v", appErr)
	}
	if sender.input.ToEmail != "" || len(repo.logs) != 0 {
		t.Fatalf("must not send or log before variable contract passes: sender=%#v logs=%#v", sender.input, repo.logs)
	}
}

func testSecretBox() secretbox.Box {
	return secretbox.New([]byte("12345678901234567890123456789012"))
}

func TestSenderErrorCodeRequiresDirectCodedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "direct coded", err: codedMailTestError{code: "DirectCode", msg: "provider"}, want: "DirectCode"},
		{name: "wrapped coded", err: errors.Join(errors.New("outer"), codedMailTestError{code: "CodeInChain", msg: "provider"}), want: verificationProviderErrorCode},
		{name: "unknown", err: errors.New("unknown provider failure"), want: verificationProviderErrorCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := senderErrorCode(tt.err); got != tt.want {
				t.Fatalf("senderErrorCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerificationCodeStatusPrecedence(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	service := &Service{diagnosticBox: testDiagnosticBox()}
	tests := []struct {
		name      string
		status    int
		expiresAt time.Time
		want      string
	}{
		{name: "failed wins after expiry", status: enum.MailLogStatusFailed, expiresAt: now.Add(-time.Minute), want: VerificationCodeStatusSendFailed},
		{name: "future failed is send failed", status: enum.MailLogStatusFailed, expiresAt: now.Add(time.Minute), want: VerificationCodeStatusSendFailed},
		{name: "exact deadline is expired", status: enum.MailLogStatusPending, expiresAt: now, want: VerificationCodeStatusExpired},
		{name: "past pending is expired", status: enum.MailLogStatusPending, expiresAt: now.Add(-time.Second), want: VerificationCodeStatusExpired},
		{name: "future pending is sending", status: enum.MailLogStatusPending, expiresAt: now.Add(time.Second), want: VerificationCodeStatusSending},
		{name: "future success is not expired", status: enum.MailLogStatusSuccess, expiresAt: now.Add(time.Second), want: VerificationCodeStatusNotExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := diagnosticLogReadRow(t, tt.status, tt.expiresAt, "diag-current", "123456")
			got, err := service.logDTOFromReadRow(row, now)
			if err != nil {
				t.Fatalf("logDTOFromReadRow returned error: %v", err)
			}
			if got.VerificationCodeStatus == nil || *got.VerificationCodeStatus != tt.want {
				t.Fatalf("status = %#v, want %q", got.VerificationCodeStatus, tt.want)
			}
		})
	}
}

func TestLogDTOFromReadRowSupportsHistoricalCurrentAndPreviousSnapshots(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	t.Run("historical row is explicitly null without diagnostic box", func(t *testing.T) {
		got, err := (&Service{}).logDTOFromReadRow(LogReadRow{Log: Log{ID: 41, Status: enum.MailLogStatusSuccess}}, now)
		if err != nil {
			t.Fatalf("historical row returned error: %v", err)
		}
		if got.VerificationCode != nil || got.VerificationCodeStatus != nil || got.VerificationCodeExpiresAt != nil {
			t.Fatalf("historical verification fields must all be nil: %#v", got)
		}
		body, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal historical log DTO: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal historical log DTO: %v", err)
		}
		for _, key := range []string{"verification_code", "verification_code_status", "verification_code_expires_at"} {
			value, exists := payload[key]
			if !exists || value != nil {
				t.Fatalf("%s must be present as JSON null, payload=%s", key, body)
			}
		}
	})

	service := &Service{diagnosticBox: testDiagnosticBox()}
	for _, keyID := range []string{"diag-current", "diag-previous"} {
		t.Run(keyID, func(t *testing.T) {
			expiresAt := now.Add(5 * time.Minute)
			row := diagnosticLogReadRow(t, enum.MailLogStatusSuccess, expiresAt, keyID, "654321")
			got, err := service.logDTOFromReadRow(row, now)
			if err != nil {
				t.Fatalf("logDTOFromReadRow returned error: %v", err)
			}
			if got.VerificationCode == nil || *got.VerificationCode != "654321" ||
				got.VerificationCodeStatus == nil || *got.VerificationCodeStatus != VerificationCodeStatusNotExpired ||
				got.VerificationCodeExpiresAt == nil || *got.VerificationCodeExpiresAt != expiresAt.Format(timeLayout) {
				t.Fatalf("unexpected verification projection: %#v", got)
			}
		})
	}
}

func TestLogDTOFromReadRowRejectsPartialVerificationJoins(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	valid := diagnosticLogReadRow(t, enum.MailLogStatusSuccess, now.Add(time.Minute), "diag-current", "123456")
	service := &Service{diagnosticBox: testDiagnosticBox()}

	for mask := 1; mask < 15; mask++ {
		row := valid
		if mask&1 == 0 {
			row.VerificationSnapshotID = nil
		}
		if mask&2 == 0 {
			row.VerificationKeyID = nil
		}
		if mask&4 == 0 {
			row.VerificationCodeEnc = nil
		}
		if mask&8 == 0 {
			row.VerificationExpiresAt = nil
		}
		t.Run(string(rune('a'+mask)), func(t *testing.T) {
			got, err := service.logDTOFromReadRow(row, now)
			assertInvalidDiagnosticProjection(t, got, err)
		})
	}
}

func TestLogDTOFromReadRowRejectsInvalidSnapshotsWithoutLeakingSecrets(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	valid := diagnosticLogReadRow(t, enum.MailLogStatusSuccess, now.Add(time.Minute), "diag-current", "123456")
	zeroID := valid
	zeroID.VerificationSnapshotID = valuePtr(uint64(0))
	unknownStatus := valid
	unknownStatus.Status = 99
	zeroExpiry := valid
	zeroExpiry.VerificationExpiresAt = valuePtr(time.Time{})
	subsecondExpiry := valid
	subsecondExpiry.VerificationExpiresAt = valuePtr(now.Add(time.Minute + time.Nanosecond))
	unknownKey := valid
	unknownKey.VerificationKeyID = valuePtr("sensitive-unknown-key")
	corruptCipher := valid
	corruptCipher.VerificationCodeEnc = valuePtr("sensitive-corrupt-ciphertext")
	emptyKey := valid
	emptyKey.VerificationKeyID = valuePtr("")
	spaceKey := valid
	spaceKey.VerificationKeyID = valuePtr("   ")
	paddedKey := valid
	paddedKey.VerificationKeyID = valuePtr(" diag-current ")
	emptyCipher := valid
	emptyCipher.VerificationCodeEnc = valuePtr("")
	spaceCipher := valid
	spaceCipher.VerificationCodeEnc = valuePtr("   ")
	paddedCipher := valid
	paddedCipher.VerificationCodeEnc = valuePtr(" " + *valid.VerificationCodeEnc + " ")
	lineWrappedCipher := valid
	lineWrappedCipher.VerificationCodeEnc = valuePtr("\r\n" + *valid.VerificationCodeEnc + "\r\n")
	lineWrappedKey := valid
	lineWrappedKey.VerificationKeyID = valuePtr("\r\ndiag-current\r\n")

	tests := []struct {
		name    string
		service *Service
		row     LogReadRow
		secrets []string
	}{
		{name: "zero child id", service: &Service{diagnosticBox: testDiagnosticBox()}, row: zeroID},
		{name: "unknown parent status", service: &Service{diagnosticBox: testDiagnosticBox()}, row: unknownStatus},
		{name: "zero expiry", service: &Service{diagnosticBox: testDiagnosticBox()}, row: zeroExpiry},
		{name: "subsecond expiry", service: &Service{diagnosticBox: testDiagnosticBox()}, row: subsecondExpiry},
		{name: "diagnostic box missing", service: &Service{}, row: valid},
		{name: "unknown key", service: &Service{diagnosticBox: testDiagnosticBox()}, row: unknownKey, secrets: []string{"sensitive-unknown-key"}},
		{name: "corrupt ciphertext", service: &Service{diagnosticBox: testDiagnosticBox()}, row: corruptCipher, secrets: []string{"sensitive-corrupt-ciphertext"}},
		{name: "empty key", service: &Service{diagnosticBox: testDiagnosticBox()}, row: emptyKey},
		{name: "whitespace key", service: &Service{diagnosticBox: testDiagnosticBox()}, row: spaceKey},
		{name: "padded key is not an alias", service: &Service{diagnosticBox: testDiagnosticBox()}, row: paddedKey, secrets: []string{"diag-current"}},
		{name: "empty ciphertext", service: &Service{diagnosticBox: testDiagnosticBox()}, row: emptyCipher},
		{name: "whitespace ciphertext", service: &Service{diagnosticBox: testDiagnosticBox()}, row: spaceCipher},
		{name: "padded ciphertext is not normalized", service: &Service{diagnosticBox: testDiagnosticBox()}, row: paddedCipher, secrets: []string{*valid.VerificationCodeEnc}},
		{name: "line wrapped ciphertext is not an alias", service: &Service{diagnosticBox: testDiagnosticBox()}, row: lineWrappedCipher, secrets: []string{*valid.VerificationCodeEnc}},
		{name: "line wrapped key is not an alias", service: &Service{diagnosticBox: testDiagnosticBox()}, row: lineWrappedKey, secrets: []string{"diag-current"}},
	}

	for _, code := range []string{"", "12345", "1234567", " 12345", "12345 ", "12345\n", "１２３４５６"} {
		row := diagnosticLogReadRow(t, enum.MailLogStatusSuccess, now.Add(time.Minute), "diag-current", code)
		tests = append(tests, struct {
			name    string
			service *Service
			row     LogReadRow
			secrets []string
		}{name: "invalid plaintext " + code, service: &Service{diagnosticBox: testDiagnosticBox()}, row: row, secrets: []string{code}})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.service.logDTOFromReadRow(tt.row, now)
			assertInvalidDiagnosticProjection(t, got, err)
			for _, secret := range tt.secrets {
				if secret != "" && strings.Contains(err.Error(), secret) {
					t.Fatalf("projector error leaked %q: %v", secret, err)
				}
			}
		})
	}
}

func TestLogDTOFromReadRowServiceReadsAreAtomicSafeAndUseOneNow(t *testing.T) {
	baseNow := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	validOne := diagnosticLogReadRow(t, enum.MailLogStatusPending, baseNow.Add(time.Minute), "diag-current", "123456")
	validOne.ID = 1
	validTwo := diagnosticLogReadRow(t, enum.MailLogStatusPending, baseNow.Add(time.Minute), "diag-current", "654321")
	validTwo.ID = 2

	t.Run("logs use one page timestamp", func(t *testing.T) {
		counter := &countingMailClock{values: []time.Time{baseNow, baseNow.Add(2 * time.Minute)}}
		service := NewServiceWithDependencies(ServiceDependencies{
			Repository:    &fakeMailRepository{listedLogRows: []LogReadRow{validOne, validTwo}},
			DiagnosticBox: testDiagnosticBox(), Clock: counter,
		})
		result, appErr := service.Logs(context.Background(), LogQuery{})
		if appErr != nil {
			t.Fatalf("Logs returned error: %v", appErr)
		}
		if counter.calls != 1 {
			t.Fatalf("Logs clock calls = %d, want 1", counter.calls)
		}
		if len(result.List) != 2 {
			t.Fatalf("Logs list length = %d, want 2", len(result.List))
		}
		for _, item := range result.List {
			if item.VerificationCodeStatus == nil || *item.VerificationCodeStatus != VerificationCodeStatusSending {
				t.Fatalf("page rows must share the first timestamp: %#v", result.List)
			}
		}
	})

	t.Run("log uses one timestamp and preserves template projection", func(t *testing.T) {
		templateID := uint64(71)
		row := validOne
		row.TemplateID = &templateID
		counter := &countingMailClock{values: []time.Time{baseNow, baseNow.Add(2 * time.Minute)}}
		repo := &fakeMailRepository{
			logRows: map[uint64]LogReadRow{row.ID: row},
			templates: map[string]*Template{"login": {
				ID: templateID, Scene: enum.VerifyCodeSceneLogin, VariablesJSON: `["code","ttl_minutes"]`, Status: enum.CommonYes,
			}},
		}
		service := NewServiceWithDependencies(ServiceDependencies{Repository: repo, DiagnosticBox: testDiagnosticBox(), Clock: counter})
		result, appErr := service.Log(context.Background(), row.ID)
		if appErr != nil {
			t.Fatalf("Log returned error: %v", appErr)
		}
		if counter.calls != 1 {
			t.Fatalf("Log clock calls = %d, want 1", counter.calls)
		}
		if result.VerificationCodeStatus == nil || *result.VerificationCodeStatus != VerificationCodeStatusSending || result.Template == nil || result.Template.ID != templateID {
			t.Fatalf("unexpected log detail: %#v", result)
		}
	})

	t.Run("invalid list row rejects whole response with safe error", func(t *testing.T) {
		invalid := diagnosticLogReadRow(t, enum.MailLogStatusSuccess, baseNow.Add(time.Minute), "diag-current", "65432x")
		invalid.ID = 2
		counter := &countingMailClock{values: []time.Time{baseNow}}
		service := NewServiceWithDependencies(ServiceDependencies{
			Repository:    &fakeMailRepository{listedLogRows: []LogReadRow{validOne, invalid}},
			DiagnosticBox: testDiagnosticBox(), Clock: counter,
		})
		result, appErr := service.Logs(context.Background(), LogQuery{})
		if result != nil {
			t.Fatalf("Logs returned partial response: %#v", result)
		}
		assertSafeDiagnosticReadError(t, appErr, "65432x", *invalid.VerificationCodeEnc, *invalid.VerificationKeyID)
		if counter.calls != 1 {
			t.Fatalf("failed Logs clock calls = %d, want 1", counter.calls)
		}
	})

	t.Run("invalid detail rejects response with safe error", func(t *testing.T) {
		invalid := validOne
		invalid.VerificationKeyID = valuePtr("sensitive-missing-key")
		counter := &countingMailClock{values: []time.Time{baseNow}}
		service := NewServiceWithDependencies(ServiceDependencies{
			Repository:    &fakeMailRepository{logRows: map[uint64]LogReadRow{invalid.ID: invalid}},
			DiagnosticBox: testDiagnosticBox(), Clock: counter,
		})
		result, appErr := service.Log(context.Background(), invalid.ID)
		if result != nil {
			t.Fatalf("Log returned partial response: %#v", result)
		}
		assertSafeDiagnosticReadError(t, appErr, "sensitive-missing-key", *invalid.VerificationCodeEnc, "123456")
		if counter.calls != 1 {
			t.Fatalf("failed Log clock calls = %d, want 1", counter.calls)
		}
	})
}

func TestLogDTOFromReadRowServiceReadsUseNilSafeClock(t *testing.T) {
	expiresAt := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	row := diagnosticLogReadRow(t, enum.MailLogStatusPending, expiresAt, "diag-current", "123456")

	t.Run("logs", func(t *testing.T) {
		service := &Service{
			repository:    &fakeMailRepository{listedLogRows: []LogReadRow{row}},
			diagnosticBox: testDiagnosticBox(),
		}
		result, appErr := service.Logs(context.Background(), LogQuery{})
		if appErr != nil || len(result.List) != 1 {
			t.Fatalf("Logs result=%#v err=%#v", result, appErr)
		}
	})

	t.Run("log", func(t *testing.T) {
		service := &Service{
			repository:    &fakeMailRepository{logRows: map[uint64]LogReadRow{row.ID: row}},
			diagnosticBox: testDiagnosticBox(),
		}
		result, appErr := service.Log(context.Background(), row.ID)
		if appErr != nil || result == nil {
			t.Fatalf("Log result=%#v err=%#v", result, appErr)
		}
	})
}

func TestLogDTOFromReadRowPageInitPublishesClosedStatusDictionary(t *testing.T) {
	result, appErr := (&Service{}).PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("PageInit returned error: %v", appErr)
	}
	want := []struct {
		label string
		value string
	}{
		{label: "发送中", value: VerificationCodeStatusSending},
		{label: "未过期", value: VerificationCodeStatusNotExpired},
		{label: "已过期", value: VerificationCodeStatusExpired},
		{label: "发送失败", value: VerificationCodeStatusSendFailed},
	}
	if len(result.Dict.MailVerificationCodeStatusArr) != len(want) {
		t.Fatalf("verification status options = %#v", result.Dict.MailVerificationCodeStatusArr)
	}
	for i, option := range result.Dict.MailVerificationCodeStatusArr {
		if option.Label != want[i].label || option.Value != want[i].value {
			t.Fatalf("option %d = %#v, want %#v", i, option, want[i])
		}
	}
}

type countingMailClock struct {
	values []time.Time
	calls  int
}

func (c *countingMailClock) Now() time.Time {
	index := c.calls
	c.calls++
	if len(c.values) == 0 {
		return time.Time{}
	}
	if index >= len(c.values) {
		index = len(c.values) - 1
	}
	return c.values[index]
}

func diagnosticLogReadRow(t *testing.T, status int, expiresAt time.Time, keyID, code string) LogReadRow {
	t.Helper()
	keys := map[string][]byte{
		"diag-current":  []byte("12345678901234567890123456789012"),
		"diag-previous": []byte("abcdefghijklmnopqrstuvwxyzABCDEF"),
	}
	key, exists := keys[keyID]
	if !exists {
		t.Fatalf("test key %q is not configured", keyID)
	}
	ciphertext, err := secretbox.New(key).Encrypt(code)
	if err != nil {
		t.Fatalf("encrypt diagnostic code: %v", err)
	}
	return LogReadRow{
		Log:                    Log{ID: 41, Status: status},
		VerificationSnapshotID: valuePtr(uint64(91)),
		VerificationKeyID:      valuePtr(keyID),
		VerificationCodeEnc:    valuePtr(ciphertext),
		VerificationExpiresAt:  valuePtr(expiresAt),
	}
}

func valuePtr[T any](value T) *T {
	return &value
}

func assertInvalidDiagnosticProjection(t *testing.T, got LogDTO, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidDiagnosticSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidDiagnosticSnapshot", err)
	}
	if got.ID != 41 {
		t.Fatalf("projector must retain only the parent DTO on failure: %#v", got)
	}
	if got.VerificationCode != nil || got.VerificationCodeStatus != nil || got.VerificationCodeExpiresAt != nil {
		t.Fatalf("projector returned partial verification data: %#v", got)
	}
}

func assertSafeDiagnosticReadError(t *testing.T, appErr interface {
	Error() string
}, secrets ...string) {
	t.Helper()
	err, ok := appErr.(*apperror.Error)
	if !ok || err == nil {
		t.Fatalf("error = %#v, want internal app error", appErr)
	}
	if err.HTTPStatus != http.StatusInternalServerError || err.LegacyCode != apperror.CodeInternal || err.Message != "读取邮件日志验证码失败" || !errors.Is(err.Cause, ErrInvalidDiagnosticSnapshot) {
		t.Fatalf("unexpected diagnostic read error: %#v", err)
	}
	metadata, marshalErr := json.Marshal(err.TemplateData)
	if marshalErr != nil {
		t.Fatalf("marshal error metadata: %v", marshalErr)
	}
	cause := ""
	if err.Cause != nil {
		cause = err.Cause.Error()
	}
	surface := strings.Join([]string{err.Error(), err.MessageID, err.Operation, string(metadata), cause}, "|")
	for _, secret := range secrets {
		if secret != "" && strings.Contains(surface, secret) {
			t.Fatalf("app error leaked %q: %s", secret, surface)
		}
	}
}

var _ Repository = (*fakeMailRepository)(nil)
var _ Sender = (*fakeMailSender)(nil)
