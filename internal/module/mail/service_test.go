package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
)

type fakeMailRepository struct {
	config    *Config
	templates map[string]*Template
	logs      map[uint64]Log
	created   Log
	nextLogID uint64
	saved     *Config
	finish    LogFinish
	finishID  uint64
	testAt    *time.Time
	testError string
	err       error
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
	if f.logs == nil {
		f.logs = map[uint64]Log{}
	}
	f.nextLogID++
	row.ID = f.nextLogID
	f.logs[row.ID] = row
	f.created = row
	return row.ID, f.err
}

func (f *fakeMailRepository) FinishLog(ctx context.Context, id uint64, finish LogFinish) error {
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
	return f.err
}

func (f *fakeMailRepository) ListLogs(ctx context.Context, query LogQuery) ([]Log, int64, error) {
	rows := make([]Log, 0, len(f.logs))
	for _, row := range f.logs {
		rows = append(rows, row)
	}
	return rows, int64(len(rows)), f.err
}

func (f *fakeMailRepository) LogByID(ctx context.Context, id uint64) (*Log, error) {
	row, ok := f.logs[id]
	if !ok {
		return nil, f.err
	}
	return &row, f.err
}

func (f *fakeMailRepository) SoftDeleteLogs(ctx context.Context, ids []uint64) error { return f.err }

type fakeMailSender struct {
	input  SendInput
	result SendResult
	err    error
}

func (f *fakeMailSender) Send(ctx context.Context, input SendInput) (SendResult, error) {
	f.input = input
	if f.err != nil {
		return SendResult{}, f.err
	}
	return f.result, nil
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
			enum.VerifyCodeSceneLogin: {ID: 77, Scene: enum.VerifyCodeSceneLogin, Subject: "Login code", TencentTemplateID: 123456, VariablesJSON: `["app_name","code","ttl_minutes"]`, SampleVariablesJSON: `{"app_name":"admin_go","code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes},
		},
	}
	sender := &fakeMailSender{result: SendResult{RequestID: "req-1", MessageID: "msg-1"}}
	service := NewService(repo, box, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "user@example.com", "654321", 5*time.Minute)

	if appErr != nil {
		t.Fatalf("expected SendVerifyCode to succeed, got %v", appErr)
	}
	if sender.input.SecretID != "AKID-secret" || sender.input.SecretKey != "SECRET-key" {
		t.Fatalf("sender must receive decrypted credentials, got %#v", sender.input)
	}
	if sender.input.TemplateID != 123456 || sender.input.TemplateData["code"] != "654321" || sender.input.TemplateData["ttl_minutes"] != "5" {
		t.Fatalf("unexpected sender template payload: %#v", sender.input)
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
			enum.VerifyCodeSceneForget: {ID: 78, Scene: enum.VerifyCodeSceneForget, Subject: "Reset code", TencentTemplateID: 123457, VariablesJSON: `["app_name","code","ttl_minutes"]`, SampleVariablesJSON: `{"app_name":"admin_go","code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes},
		},
	}
	sender := &fakeMailSender{err: codedMailTestError{code: "FailedOperation.TemplateNotApproved", msg: "template not approved"}}
	service := NewService(repo, box, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneForget, "user@example.com", "654321", 5*time.Minute)

	if appErr == nil || appErr.Message != "邮件发送失败" {
		t.Fatalf("expected send failure, got %#v", appErr)
	}
	if repo.finish.Status != enum.MailLogStatusFailed || repo.finish.ErrorCode != "FailedOperation.TemplateNotApproved" || repo.finish.ErrorMessage != "template not approved" {
		t.Fatalf("unexpected failed finish log: %#v", repo.finish)
	}
	if strings.Contains(repo.finish.ErrorMessage, "654321") || strings.Contains(repo.finish.ErrorMessage, "TemplateData") {
		t.Fatalf("failed log must not persist verify code or template data: %#v", repo.finish)
	}
}

func TestServiceConfigResponseDoesNotExposeEncryptedSecrets(t *testing.T) {
	repo := &fakeMailRepository{config: &Config{ID: 1, SecretIDEnc: "cipher-id", SecretIDHint: "***t-id", SecretKeyEnc: "cipher-key", SecretKeyHint: "***-key", Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes}}
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
		FromEmail: "noreply@example.com", Status: enum.CommonYes,
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

	appErr := service.SaveConfig(context.Background(), SaveConfigInput{Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes})
	if appErr == nil || !strings.Contains(appErr.Message, "首次配置必须填写") {
		t.Fatalf("expected first config secret error, got %#v", appErr)
	}

	existingID, _ := box.Encrypt("AKID-existing")
	existingKey, _ := box.Encrypt("SECRET-existing")
	repo := &fakeMailRepository{config: &Config{SecretIDEnc: existingID, SecretIDHint: "***ting", SecretKeyEnc: existingKey, SecretKeyHint: "***ting", Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "old@example.com", Status: enum.CommonYes}}
	service = NewService(repo, box, &fakeMailSender{})

	appErr = service.SaveConfig(context.Background(), SaveConfigInput{Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "new@example.com", Status: enum.CommonYes})
	if appErr != nil {
		t.Fatalf("expected edit to reuse existing secrets, got %v", appErr)
	}
	if repo.saved == nil || repo.saved.SecretIDEnc != existingID || repo.saved.SecretKeyEnc != existingKey || repo.saved.FromEmail != "new@example.com" {
		t.Fatalf("unexpected saved config: %#v", repo.saved)
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
	service := NewService(repo, box, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneBindEmail, "user@example.com", "654321", 5*time.Minute)
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
