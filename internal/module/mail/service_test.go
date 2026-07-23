package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"
)

type fakeMailRepository struct {
	config              *Config
	templates           map[string]*Template
	logs                map[uint64]Log
	logRows             map[uint64]LogReadRow
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
		"diag-current": []byte("12345678901234567890123456789012"),
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

func TestSendVerifyCodeSkipsProviderWhenDeadlineExpiresAfterCommit(t *testing.T) {
	credentialBox := testSecretBox()
	repo := configuredVerifyRepository(credentialBox, enum.VerifyCodeSceneLogin)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	reads := 0
	sender := &fakeMailSender{}
	service := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: credentialBox, DiagnosticBox: testDiagnosticBox(), Sender: sender,
		Clock: clock.Func(func() time.Time {
			reads++
			if reads > 1 {
				return now.Add(5 * time.Minute)
			}
			return now
		}),
	})

	err := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute, now.Add(5*time.Minute))

	if err == nil || err.Message != "验证码发送已过期" {
		t.Fatalf("expected safe deadline error, got %#v", err)
	}
	if sender.calls != 0 || repo.finish.Status != enum.MailLogStatusFailed || repo.finish.ErrorCode != "verification_deadline_elapsed" || repo.createdVerification == nil {
		t.Fatalf("deadline must finalize parent without provider and preserve child: calls=%d finish=%#v child=%#v", sender.calls, repo.finish, repo.createdVerification)
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

func TestSenderErrorCodeUsesErrorsAs(t *testing.T) {
	wrapped := errors.Join(errors.New("outer"), codedMailTestError{code: "CodeInChain", msg: "provider"})
	if got := senderErrorCode(wrapped); got != "CodeInChain" {
		t.Fatalf("expected coded error through errors.As, got %q", got)
	}
}

var _ Repository = (*fakeMailRepository)(nil)
var _ Sender = (*fakeMailSender)(nil)
