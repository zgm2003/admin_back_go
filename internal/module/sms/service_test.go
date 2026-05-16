package sms

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/platform/secretbox"
)

func TestPageInitKeepsSmsScenesDomesticAndBounded(t *testing.T) {
	result, appErr := NewService(nil, secretbox.Box{}, nil).PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("PageInit error = %v", appErr)
	}
	if len(result.Dict.SmsRegionArr) != 1 || result.Dict.SmsRegionArr[0].Value != DefaultRegion {
		t.Fatalf("sms regions = %#v", result.Dict.SmsRegionArr)
	}
	for _, item := range result.Dict.SmsSceneArr {
		if item.Value == enum.VerifyCodeSceneBindEmail {
			t.Fatalf("sms scenes must not include email scene: %#v", result.Dict.SmsSceneArr)
		}
	}
}

func TestTemplateRowRequiresVerifyCodeVariablesOnly(t *testing.T) {
	_, appErr := templateRowFromInput(SaveTemplateInput{
		Scene: enum.VerifyCodeSceneLogin, Name: "登录验证码", TencentTemplateID: "12345",
		Variables:       []string{"code", "ttl_minutes", "app_name"},
		SampleVariables: map[string]string{"code": "123456", "ttl_minutes": "5", "app_name": "Admin"},
		Status:          enum.CommonYes,
	})
	if appErr == nil {
		t.Fatal("expected extra variable to be rejected")
	}

	row, appErr := templateRowFromInput(SaveTemplateInput{
		Scene: enum.VerifyCodeSceneBindPhone, Name: "绑定手机", TencentTemplateID: "12345",
		Variables:       []string{"ttl_minutes", "code"},
		SampleVariables: map[string]string{"code": "123456", "ttl_minutes": "5"},
		Status:          enum.CommonYes,
	})
	if appErr != nil {
		t.Fatalf("templateRowFromInput valid input: %v", appErr)
	}
	if row.Scene != enum.VerifyCodeSceneBindPhone || row.TencentTemplateID != "12345" {
		t.Fatalf("unexpected row: %#v", row)
	}
}

func TestNormalizePhoneSupportsDomesticSinglePhoneOnly(t *testing.T) {
	cases := map[string]string{
		"13800138000":      "+8613800138000",
		"+8613800138000":   "+8613800138000",
		"86 138-0013-8000": "+8613800138000",
	}
	for input, want := range cases {
		got, appErr := normalizePhone(input)
		if appErr != nil || got != want {
			t.Fatalf("normalizePhone(%q) = %q, %v; want %q", input, got, appErr, want)
		}
	}
	if _, appErr := normalizePhone("+85261234567"); appErr == nil {
		t.Fatal("expected non-mainland phone to be rejected")
	}
}

func TestTestSendCreatesPendingLogAndFinishesSuccessWithoutSensitivePayload(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	secretID, err := box.Encrypt("AKID")
	if err != nil {
		t.Fatal(err)
	}
	secretKey, err := box.Encrypt("SECRET")
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeSmsRepository()
	repo.config = &Config{
		ID: 1, SecretIDEnc: secretID, SecretKeyEnc: secretKey, SmsSdkAppID: "1400000000", SignName: "签名",
		Region: DefaultRegion, Endpoint: DefaultEndpoint, Status: enum.CommonYes, IsDel: enum.CommonNo,
	}
	repo.templates[enum.VerifyCodeSceneLogin] = &Template{
		ID: 7, Scene: enum.VerifyCodeSceneLogin, Name: "登录验证码", TencentTemplateID: "12345",
		VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	}
	sender := &fakeSmsSender{result: SendResult{RequestID: "req-1", SerialNo: "serial-1", Fee: 1}}
	service := NewService(repo, box, sender)

	appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})
	if appErr != nil {
		t.Fatalf("TestSend error = %v", appErr)
	}
	if len(repo.createdLogs) != 1 {
		t.Fatalf("created logs = %#v", repo.createdLogs)
	}
	created := repo.createdLogs[0]
	if created.Scene != enum.SmsSceneTest || created.Status != enum.SmsLogStatusPending || created.ToPhone != "+8613800138000" || created.TemplateID == nil || *created.TemplateID != 7 {
		t.Fatalf("pending log mismatch: %#v", created)
	}
	if created.ErrorMessage != "" || created.TencentRequestID != "" || created.TencentSerialNo != "" {
		t.Fatalf("pending log must not contain payload/result fields: %#v", created)
	}
	if !reflect.DeepEqual(sender.input.TemplateParamSet, []string{"123456", "5"}) {
		t.Fatalf("template params = %#v", sender.input.TemplateParamSet)
	}
	finish := repo.finishes[1]
	if finish.Status != enum.SmsLogStatusSuccess || finish.RequestID != "req-1" || finish.SerialNo != "serial-1" || finish.Fee != 1 || finish.SentAt == nil {
		t.Fatalf("finish mismatch: %#v", finish)
	}
	if repo.lastTestError != "" || repo.lastTestAt == nil {
		t.Fatalf("test result mismatch: at=%v err=%q", repo.lastTestAt, repo.lastTestError)
	}
}

func TestTestSendFinishesFailedLogWithRequestIDFromSenderError(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	secretID, _ := box.Encrypt("AKID")
	secretKey, _ := box.Encrypt("SECRET")
	repo := newFakeSmsRepository()
	repo.config = &Config{ID: 1, SecretIDEnc: secretID, SecretKeyEnc: secretKey, SmsSdkAppID: "1400000000", SignName: "签名", Region: DefaultRegion, Endpoint: DefaultEndpoint, Status: enum.CommonYes, IsDel: enum.CommonNo}
	repo.templates[enum.VerifyCodeSceneLogin] = &Template{ID: 7, Scene: enum.VerifyCodeSceneLogin, Name: "登录验证码", TencentTemplateID: "12345", VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes, IsDel: enum.CommonNo}
	sender := &fakeSmsSender{result: SendResult{RequestID: "req-fail", SerialNo: "serial-fail", Fee: 1}, err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "template incorrect"}}
	service := NewService(repo, box, sender)

	appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})
	if appErr == nil {
		t.Fatal("expected send failure")
	}
	finish := repo.finishes[1]
	if finish.Status != enum.SmsLogStatusFailed || finish.RequestID != "req-fail" || finish.SerialNo != "serial-fail" || finish.ErrorCode != "FailedOperation.TemplateIncorrect" {
		t.Fatalf("failed finish mismatch: %#v", finish)
	}
	if repo.lastTestError == "" || repo.lastTestAt == nil {
		t.Fatalf("failure test result not recorded: at=%v err=%q", repo.lastTestAt, repo.lastTestError)
	}
}

type fakeSmsSender struct {
	input  SendInput
	result SendResult
	err    error
}

func (f *fakeSmsSender) Send(ctx context.Context, input SendInput) (SendResult, error) {
	f.input = input
	return f.result, f.err
}

type fakeCodedError struct {
	code    string
	message string
}

func (e fakeCodedError) Error() string     { return e.message }
func (e fakeCodedError) ErrorCode() string { return e.code }

type fakeSmsRepository struct {
	config        *Config
	templates     map[string]*Template
	logs          map[uint64]*Log
	createdLogs   []Log
	finishes      map[uint64]LogFinish
	setting       *systemsetting.Setting
	lastTestAt    *time.Time
	lastTestError string
	nextID        uint64
}

func newFakeSmsRepository() *fakeSmsRepository {
	return &fakeSmsRepository{
		templates: map[string]*Template{},
		logs:      map[uint64]*Log{},
		finishes:  map[uint64]LogFinish{},
		setting:   &systemsetting.Setting{SettingKey: verifyCodeTTLSettingKey, SettingValue: "5", Status: enum.CommonYes, IsDel: enum.CommonNo},
		nextID:    1,
	}
}

func (r *fakeSmsRepository) DefaultConfig(ctx context.Context) (*Config, error) { return r.config, nil }
func (r *fakeSmsRepository) SaveDefaultConfig(ctx context.Context, row Config) error {
	r.config = &row
	return nil
}
func (r *fakeSmsRepository) SoftDeleteDefaultConfig(ctx context.Context) error {
	if r.config != nil {
		r.config.IsDel = enum.CommonYes
	}
	return nil
}
func (r *fakeSmsRepository) UpdateConfigTestResult(ctx context.Context, at *time.Time, errorMessage string) error {
	r.lastTestAt = at
	r.lastTestError = errorMessage
	return nil
}
func (r *fakeSmsRepository) ListTemplates(ctx context.Context) ([]Template, error) {
	rows := make([]Template, 0, len(r.templates))
	for _, row := range r.templates {
		rows = append(rows, *row)
	}
	return rows, nil
}
func (r *fakeSmsRepository) TemplateByID(ctx context.Context, id uint64) (*Template, error) {
	for _, row := range r.templates {
		if row.ID == id {
			return row, nil
		}
	}
	return nil, nil
}
func (r *fakeSmsRepository) TemplateByScene(ctx context.Context, scene string) (*Template, error) {
	return r.templates[scene], nil
}
func (r *fakeSmsRepository) SaveTemplate(ctx context.Context, row Template) (uint64, error) {
	if row.ID == 0 {
		row.ID = r.nextID
		r.nextID++
	}
	copied := row
	r.templates[row.Scene] = &copied
	return row.ID, nil
}
func (r *fakeSmsRepository) UpdateTemplate(ctx context.Context, id uint64, update TemplateUpdate) error {
	for _, row := range r.templates {
		if row.ID == id {
			row.Scene = update.Scene
			row.Name = update.Name
			row.TencentTemplateID = update.TencentTemplateID
			row.VariablesJSON = update.VariablesJSON
			row.SampleVariablesJSON = update.SampleVariablesJSON
			row.Status = update.Status
			return nil
		}
	}
	return errors.New("not found")
}
func (r *fakeSmsRepository) SoftDeleteTemplate(ctx context.Context, id uint64) error {
	row, _ := r.TemplateByID(ctx, id)
	if row != nil {
		row.IsDel = enum.CommonYes
	}
	return nil
}
func (r *fakeSmsRepository) CreateLog(ctx context.Context, row Log) (uint64, error) {
	row.ID = r.nextID
	r.nextID++
	copied := row
	r.logs[row.ID] = &copied
	r.createdLogs = append(r.createdLogs, row)
	return row.ID, nil
}
func (r *fakeSmsRepository) FinishLog(ctx context.Context, id uint64, finish LogFinish) error {
	r.finishes[id] = finish
	if row := r.logs[id]; row != nil {
		row.Status = finish.Status
		row.TencentRequestID = finish.RequestID
		row.TencentSerialNo = finish.SerialNo
		row.TencentFee = finish.Fee
		row.ErrorCode = finish.ErrorCode
		row.ErrorMessage = finish.ErrorMessage
		row.DurationMS = finish.DurationMS
		row.SentAt = finish.SentAt
	}
	return nil
}
func (r *fakeSmsRepository) ListLogs(ctx context.Context, query LogQuery) ([]Log, int64, error) {
	rows := make([]Log, 0, len(r.logs))
	for _, row := range r.logs {
		rows = append(rows, *row)
	}
	return rows, int64(len(rows)), nil
}
func (r *fakeSmsRepository) LogByID(ctx context.Context, id uint64) (*Log, error) {
	return r.logs[id], nil
}
func (r *fakeSmsRepository) SoftDeleteLogs(ctx context.Context, ids []uint64) error {
	for _, id := range ids {
		if row := r.logs[id]; row != nil {
			row.IsDel = enum.CommonYes
		}
	}
	return nil
}
func (r *fakeSmsRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	if key != verifyCodeTTLSettingKey {
		return nil, nil
	}
	return r.setting, nil
}
func (r *fakeSmsRepository) SaveSetting(ctx context.Context, row systemsetting.Setting) error {
	r.setting = &row
	return nil
}
func (r *fakeSmsRepository) InvalidateSettingCache(ctx context.Context, key string) error { return nil }
