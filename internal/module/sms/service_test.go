package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/shared/enum"
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

func newVerificationSMSService(t *testing.T, sender *fakeSmsSender) (*Service, *fakeSmsRepository) {
	t.Helper()
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
	repo.config = readySMSConfig()
	repo.config.SecretIDEnc = secretID
	repo.config.SecretKeyEnc = secretKey
	repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
	return NewService(repo, box, sender), repo
}

func TestSendVerifyCodeUsesCodeTTLAndFinalizesSuccess(t *testing.T) {
	sender := &fakeSmsSender{result: SendResult{RequestID: "req-1", SerialNo: "serial-1", Fee: 1}}
	service, repo := newVerificationSMSService(t, sender)
	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)
	if appErr != nil {
		t.Fatalf("SendVerifyCode error=%#v", appErr)
	}
	if !reflect.DeepEqual(sender.input.TemplateParamSet, []string{"654321", "12"}) {
		t.Fatalf("params=%#v", sender.input.TemplateParamSet)
	}
	if len(repo.createdLogs) != 1 {
		t.Fatalf("logs=%#v", repo.createdLogs)
	}
	created := repo.createdLogs[0]
	if created.Scene != enum.VerifyCodeSceneLogin || created.ToPhone != "+8613800138000" || created.Status != enum.SmsLogStatusPending {
		t.Fatalf("pending=%#v", created)
	}
	finish := repo.finishes[created.ID]
	if finish.Status != enum.SmsLogStatusSuccess || finish.RequestID != "req-1" || finish.SerialNo != "serial-1" || finish.Fee != 1 || finish.SentAt == nil {
		t.Fatalf("finish=%#v", finish)
	}
	if repo.lastTestAt != nil || repo.lastTestError != "" {
		t.Fatalf("real delivery changed test result: at=%v err=%q", repo.lastTestAt, repo.lastTestError)
	}

	loggedValues := map[string]any{"created": repo.createdLogs, "stored": repo.logs[created.ID], "finish": finish, "dto": logDTOFromRow(*repo.logs[created.ID])}
	for name, value := range loggedValues {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		for _, sensitive := range [][]byte{[]byte("654321"), []byte("AKID"), []byte("SECRET")} {
			if bytes.Contains(raw, sensitive) {
				t.Fatalf("%s contains sensitive value: %s", name, raw)
			}
		}
	}
}

func TestSendVerifyCodeFinalizesProviderFailure(t *testing.T) {
	sender := &fakeSmsSender{result: SendResult{RequestID: "req-fail", SerialNo: "serial-fail", Fee: 1}, err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "template incorrect"}}
	service, repo := newVerificationSMSService(t, sender)
	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 5*time.Minute)
	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatalf("error=%#v", appErr)
	}
	created := repo.createdLogs[0]
	finish := repo.finishes[created.ID]
	if finish.Status != enum.SmsLogStatusFailed || finish.RequestID != "req-fail" || finish.SerialNo != "serial-fail" || finish.ErrorCode != "FailedOperation.TemplateIncorrect" || finish.ErrorMessage != "template incorrect" {
		t.Fatalf("finish=%#v", finish)
	}
	if repo.lastTestAt != nil || repo.lastTestError != "" {
		t.Fatalf("real delivery changed test result: at=%v err=%q", repo.lastTestAt, repo.lastTestError)
	}
}

func TestSendVerifyCodeUsesTrustedMessageForKnownProviderCode(t *testing.T) {
	const rawMessage = "arbitrary raw provider detail"
	sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: rawMessage}}
	service, repo := newVerificationSMSService(t, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)

	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatal("expected trusted provider failure")
	}
	created := repo.createdLogs[0]
	finish := repo.finishes[created.ID]
	if finish.ErrorCode != "FailedOperation.TemplateIncorrect" || finish.ErrorMessage != "template incorrect" {
		t.Fatal("known provider code did not use its trusted message")
	}
	assertSerializedValuesExclude(t, map[string]any{
		"created": repo.createdLogs,
		"stored":  repo.logs[created.ID],
		"finish":  finish,
		"dto":     logDTOFromRow(*repo.logs[created.ID]),
	}, rawMessage)
	assertErrorChainExcludes(t, appErr, rawMessage)
}

func TestSMSDeliveryRejectsEncodedSensitiveUnknownProviderFailure(t *testing.T) {
	const (
		maliciousCode = "FailedOperation.NjU0MzIx414b4944534543524554"
		rawMessage    = "encoded N j U 0 M z I x Q U t J R A U 0 V D U k V U 363534333231 414b4944 534543524554"
	)
	for _, path := range []string{"real", "test-send"} {
		t.Run(path, func(t *testing.T) {
			sender := &fakeSmsSender{err: fakeCodedError{code: maliciousCode, message: rawMessage}}
			service, repo := newVerificationSMSService(t, sender)
			repo.config.VerifyCodeTTLMinutes = 12

			var appErr error
			if path == "real" {
				appErr = service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)
			} else {
				appErr = service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})
			}

			if appErr == nil {
				t.Fatal("expected provider failure")
			}
			created := repo.createdLogs[0]
			finish := repo.finishes[created.ID]
			if finish.ErrorCode != "" || finish.ErrorMessage != "短信发送失败" {
				t.Fatal("encoded provider failure was not reduced to generic summary")
			}
			if path == "test-send" && (repo.lastTestAt == nil || repo.lastTestError != "短信发送失败") {
				t.Fatal("encoded provider failure reached test-send result")
			}
			assertSerializedValuesExclude(t, map[string]any{
				"created":         repo.createdLogs,
				"stored":          repo.logs[created.ID],
				"finish":          finish,
				"dto":             logDTOFromRow(*repo.logs[created.ID]),
				"last_test_error": repo.lastTestError,
			}, maliciousCode, rawMessage, "NjU0MzIx", "QUtJRA", "U0VDUkVU", "363534333231", "414b4944", "534543524554")
			assertErrorChainExcludes(t, appErr, maliciousCode, rawMessage, "NjU0MzIx", "QUtJRA", "U0VDUkVU", "363534333231", "414b4944", "534543524554")
		})
	}
}

func TestSendVerifyCodeKnownProviderCodeIgnoresEncodedRawMessage(t *testing.T) {
	const rawMessage = "NjU0MzIx QUtJRA U0VDUkVU 363534333231 414b4944 534543524554 full provider payload"
	sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: rawMessage}}
	service, repo := newVerificationSMSService(t, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)

	if appErr == nil {
		t.Fatal("expected provider failure")
	}
	finish := repo.finishes[repo.createdLogs[0].ID]
	if finish.ErrorCode != "FailedOperation.TemplateIncorrect" || finish.ErrorMessage != "template incorrect" {
		t.Fatal("known provider code did not ignore malicious raw message")
	}
	assertSerializedValuesExclude(t, map[string]any{"finish": finish, "stored": repo.logs[repo.createdLogs[0].ID]}, rawMessage)
	assertErrorChainExcludes(t, appErr, rawMessage)
}

func TestSendVerifyCodeDoesNotPersistOrWrapSensitiveProviderFailure(t *testing.T) {
	const payloadMarker = "provider_payload_marker"
	sender := &fakeSmsSender{
		result: SendResult{RequestID: "req-sensitive", SerialNo: "serial-sensitive", Fee: 1},
		err: fakeCodedError{
			code:    "FailedOperation.TemplateParamSet.654321.AKID.SECRET",
			message: `{"SecretID":"AKID","SecretKey":"SECRET","TemplateParamSet":["654321","5"],"provider_payload_marker":"full-payload"}`,
		},
	}
	service, repo := newVerificationSMSService(t, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 5*time.Minute)

	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatalf("error=%#v", appErr)
	}
	created := repo.createdLogs[0]
	finish := repo.finishes[created.ID]
	if finish.ErrorCode != "" || finish.ErrorMessage != "短信发送失败" {
		t.Fatal("provider failure summary was not sanitized")
	}
	assertSerializedValuesExclude(t, map[string]any{
		"created": repo.createdLogs,
		"stored":  repo.logs[created.ID],
		"finish":  finish,
		"dto":     logDTOFromRow(*repo.logs[created.ID]),
	}, "654321", "AKID", "SECRET", "TemplateParamSet", payloadMarker, "full-payload")
	assertErrorChainExcludes(t, appErr, "654321", "AKID", "SECRET", "TemplateParamSet", payloadMarker, "full-payload")
}

func TestTestSendDoesNotPersistOrWrapSensitiveProviderFailure(t *testing.T) {
	const payloadMarker = "provider_payload_marker"
	sender := &fakeSmsSender{err: fakeCodedError{
		code:    "FailedOperation.TemplateParamSet.123456.AKID.SECRET",
		message: `{"SecretID":"AKID","SecretKey":"SECRET","TemplateParamSet":["123456","5"],"provider_payload_marker":"full-payload"}`,
	}}
	service, repo := newVerificationSMSService(t, sender)

	appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatalf("error=%#v", appErr)
	}
	if repo.lastTestAt == nil || repo.lastTestError != "短信发送失败" {
		t.Fatal("test-send failure summary was not sanitized")
	}
	assertSerializedValuesExclude(t, map[string]any{"last_test_error": repo.lastTestError},
		"123456", "AKID", "SECRET", "TemplateParamSet", payloadMarker, "full-payload")
	assertErrorChainExcludes(t, appErr, "123456", "AKID", "SECRET", "TemplateParamSet", payloadMarker, "full-payload")
}

func TestSendVerifyCodeRejectsProviderFieldNameInErrorCode(t *testing.T) {
	sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateParamSet", message: "template incorrect"}}
	service, repo := newVerificationSMSService(t, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 5*time.Minute)

	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatalf("error=%#v", appErr)
	}
	finish := repo.finishes[repo.createdLogs[0].ID]
	if finish.ErrorCode != "" || finish.ErrorMessage != "短信发送失败" {
		t.Fatal("provider field name was retained in failure summary")
	}
}

func TestSMSDeliveryUsesTrustedCatalogForObfuscatedProviderFailure(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "zero width separators",
			message: "Template_Para\u200bm_Set code 6\u200b54321 Secret_ID A\u200bKID Secret_Key SEC\u200bRET",
		},
		{
			name:    "ascii separators",
			message: "code 6 54321 Secret ID A K I D Secret Key S E C R E T",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/real", func(t *testing.T) {
			sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: tt.message}}
			service, repo := newVerificationSMSService(t, sender)

			appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)

			if appErr == nil || appErr.MessageID != "sms.send.failed" {
				t.Fatalf("error=%#v", appErr)
			}
			created := repo.createdLogs[0]
			finish := repo.finishes[created.ID]
			if finish.ErrorCode != "FailedOperation.TemplateIncorrect" || finish.ErrorMessage != "template incorrect" {
				t.Fatal("obfuscated provider failure did not use trusted catalog")
			}
			assertSerializedValuesExclude(t, map[string]any{
				"created": repo.createdLogs,
				"stored":  repo.logs[created.ID],
				"finish":  finish,
				"dto":     logDTOFromRow(*repo.logs[created.ID]),
			}, tt.message)
			assertErrorChainExcludes(t, appErr, tt.message)
		})

		t.Run(tt.name+"/test-send", func(t *testing.T) {
			sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: tt.message}}
			service, repo := newVerificationSMSService(t, sender)
			repo.config.VerifyCodeTTLMinutes = 12

			appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

			if appErr == nil || appErr.MessageID != "sms.send.failed" {
				t.Fatalf("error=%#v", appErr)
			}
			if repo.lastTestAt == nil || repo.lastTestError != "template incorrect" {
				t.Fatal("test-send did not use trusted provider catalog")
			}
			assertSerializedValuesExclude(t, map[string]any{"last_test_error": repo.lastTestError}, tt.message)
			assertErrorChainExcludes(t, appErr, tt.message)
		})
	}
}

func TestSendVerifyCodeFinalizesAfterCallerCancellation(t *testing.T) {
	tests := []struct {
		name       string
		senderErr  error
		wantStatus int
		wantKey    string
	}{
		{name: "success", wantStatus: enum.SmsLogStatusSuccess},
		{name: "provider failure", senderErr: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "untrusted"}, wantStatus: enum.SmsLogStatusFailed, wantKey: "sms.send.failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			sender := &fakeSmsSender{err: tt.senderErr, onSend: cancel}
			service, repo := newVerificationSMSService(t, sender)

			appErr := service.SendVerifyCode(ctx, enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)

			if tt.wantKey == "" && appErr != nil {
				t.Fatalf("SendVerifyCode error=%#v", appErr)
			}
			if tt.wantKey != "" && (appErr == nil || appErr.MessageID != tt.wantKey) {
				t.Fatalf("SendVerifyCode error=%#v", appErr)
			}
			if repo.finishLogCalls != 1 || repo.finishLogCtxErr != nil {
				t.Fatalf("finish calls=%d ctx_err=%v", repo.finishLogCalls, repo.finishLogCtxErr)
			}
			if finish := repo.finishes[repo.createdLogs[0].ID]; finish.Status != tt.wantStatus {
				t.Fatalf("finish=%#v", finish)
			}
		})
	}
}

func TestTestSendRecordsProviderFailureAfterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "untrusted"}, onSend: cancel}
	service, repo := newVerificationSMSService(t, sender)

	appErr := service.TestSend(ctx, TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatalf("TestSend error=%#v", appErr)
	}
	if repo.finishLogCalls != 1 || repo.finishLogCtxErr != nil {
		t.Fatalf("finish calls=%d ctx_err=%v", repo.finishLogCalls, repo.finishLogCtxErr)
	}
	if repo.updateTestResultCalls != 1 || repo.updateTestResultCtxErr != nil || repo.lastTestError != "template incorrect" {
		t.Fatalf("test result calls=%d ctx_err=%v error=%q", repo.updateTestResultCalls, repo.updateTestResultCtxErr, repo.lastTestError)
	}
}

func TestVerificationDeliveryRepositoryErrorPrecedence(t *testing.T) {
	t.Run("create log failure skips provider and finish", func(t *testing.T) {
		sender := &fakeSmsSender{}
		service, repo := newVerificationSMSService(t, sender)
		repo.createLogErr = errors.New("create log unavailable")

		appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)

		if appErr == nil || appErr.MessageID != "sms.log.create_failed" || sender.calls != 0 || repo.finishLogCalls != 0 {
			t.Fatalf("error=%#v sender_calls=%d finish_calls=%d", appErr, sender.calls, repo.finishLogCalls)
		}
	})

	t.Run("provider failure and finish failure skips test result", func(t *testing.T) {
		const rawMessage = "654321 AKID SECRET provider payload"
		sender := &fakeSmsSender{err: fakeCodedError{code: "Unknown.Provider", message: rawMessage}}
		service, repo := newVerificationSMSService(t, sender)
		repo.finishLogErr = errors.New("finish log unavailable")

		appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

		if appErr == nil || appErr.MessageID != "sms.log.finish_failed" || repo.finishLogCalls != 1 || repo.updateTestResultCalls != 0 {
			t.Fatalf("error=%#v finish_calls=%d update_calls=%d", appErr, repo.finishLogCalls, repo.updateTestResultCalls)
		}
		assertErrorChainExcludes(t, appErr, rawMessage, "654321", "AKID", "SECRET")
	})

	t.Run("provider failure ignores test result failure", func(t *testing.T) {
		sender := &fakeSmsSender{err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "untrusted"}}
		service, repo := newVerificationSMSService(t, sender)
		repo.updateTestResultErr = errors.New("test result unavailable")

		appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

		if appErr == nil || appErr.MessageID != "sms.send.failed" || repo.finishLogCalls != 1 || repo.updateTestResultCalls != 1 {
			t.Fatalf("error=%#v finish_calls=%d update_calls=%d", appErr, repo.finishLogCalls, repo.updateTestResultCalls)
		}
	})

	t.Run("success finish failure skips test result", func(t *testing.T) {
		sender := &fakeSmsSender{}
		service, repo := newVerificationSMSService(t, sender)
		repo.finishLogErr = errors.New("finish log unavailable")

		appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

		if appErr == nil || appErr.MessageID != "sms.log.finish_failed" || repo.finishLogCalls != 1 || repo.updateTestResultCalls != 0 {
			t.Fatalf("error=%#v finish_calls=%d update_calls=%d", appErr, repo.finishLogCalls, repo.updateTestResultCalls)
		}
	})

	t.Run("success test result failure is returned", func(t *testing.T) {
		sender := &fakeSmsSender{}
		service, repo := newVerificationSMSService(t, sender)
		repo.updateTestResultErr = errors.New("test result unavailable")

		appErr := service.TestSend(context.Background(), TestInput{ToPhone: "13800138000", TemplateScene: enum.VerifyCodeSceneLogin})

		if appErr == nil || appErr.MessageID != "sms.config.test_result_failed" || repo.finishLogCalls != 1 || repo.updateTestResultCalls != 1 {
			t.Fatalf("error=%#v finish_calls=%d update_calls=%d", appErr, repo.finishLogCalls, repo.updateTestResultCalls)
		}
	})
}

func TestSendVerifyCodeDropsUnsafeProviderResultMetadata(t *testing.T) {
	const (
		unsafeRequestID = `{"payload":"654321 AKID SECRET"}`
		unsafeSerialNo  = "serial-TemplateParamSet-654321-AKID-SECRET"
	)
	tests := []struct {
		name      string
		senderErr error
	}{
		{name: "success"},
		{name: "provider failure", senderErr: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "untrusted"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSmsSender{
				result: SendResult{RequestID: unsafeRequestID, SerialNo: unsafeSerialNo, Fee: 7},
				err:    tt.senderErr,
			}
			service, repo := newVerificationSMSService(t, sender)

			appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute)

			if tt.senderErr == nil && appErr != nil {
				t.Fatalf("SendVerifyCode error=%#v", appErr)
			}
			if tt.senderErr != nil && (appErr == nil || appErr.MessageID != "sms.send.failed") {
				t.Fatalf("SendVerifyCode error=%#v", appErr)
			}
			created := repo.createdLogs[0]
			finish := repo.finishes[created.ID]
			if finish.RequestID != "" || finish.SerialNo != "" || finish.Fee != 7 {
				t.Fatal("unsafe provider result metadata was retained")
			}
			assertSerializedValuesExclude(t, map[string]any{
				"stored": repo.logs[created.ID],
				"finish": finish,
				"dto":    logDTOFromRow(*repo.logs[created.ID]),
			}, unsafeRequestID, unsafeSerialNo, "654321", "AKID", "SECRET", "TemplateParamSet", "payload")
		})
	}
}

func TestSendVerifyCodePreservesProviderMetadataContainingTTL(t *testing.T) {
	tests := []struct {
		name      string
		senderErr error
	}{
		{name: "success"},
		{name: "provider failure", senderErr: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "untrusted"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSmsSender{
				result: SendResult{RequestID: "req-5", SerialNo: "serial-5", Fee: 7},
				err:    tt.senderErr,
			}
			service, repo := newVerificationSMSService(t, sender)

			appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 5*time.Minute)

			if tt.senderErr == nil && appErr != nil {
				t.Fatalf("SendVerifyCode error=%#v", appErr)
			}
			if tt.senderErr != nil && (appErr == nil || appErr.MessageID != "sms.send.failed") {
				t.Fatalf("SendVerifyCode error=%#v", appErr)
			}
			created := repo.createdLogs[0]
			finish := repo.finishes[created.ID]
			if finish.RequestID != "req-5" || finish.SerialNo != "serial-5" {
				t.Fatalf("provider metadata = request ID %q, serial no %q", finish.RequestID, finish.SerialNo)
			}
		})
	}
}

func TestSendVerifyCodeRejectsInvalidInputBeforeLogging(t *testing.T) {
	tests := []struct {
		name, scene, phone, code string
		ttl                      time.Duration
	}{
		{name: "scene", scene: enum.VerifyCodeSceneBindEmail, phone: "13800138000", code: "654321", ttl: 5 * time.Minute},
		{name: "phone", scene: enum.VerifyCodeSceneLogin, phone: "bad", code: "654321", ttl: 5 * time.Minute},
		{name: "empty code", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "", ttl: 5 * time.Minute},
		{name: "non numeric code", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "abcdef", ttl: 5 * time.Minute},
		{name: "short code", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "12345", ttl: 5 * time.Minute},
		{name: "code whitespace", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: " 123456 ", ttl: 5 * time.Minute},
		{name: "ttl zero", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "654321", ttl: 0},
		{name: "ttl low", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "654321", ttl: time.Second},
		{name: "ttl high", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "654321", ttl: 60*time.Minute + time.Nanosecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSmsSender{}
			service, repo := newVerificationSMSService(t, sender)
			appErr := service.SendVerifyCode(context.Background(), tt.scene, tt.phone, tt.code, tt.ttl)
			if appErr == nil || len(repo.createdLogs) != 0 || sender.input.PhoneNumber != "" {
				t.Fatal("invalid verification input reached logging or provider delivery")
			}
		})
	}
}

func assertSerializedValuesExclude(t *testing.T, values map[string]any, forbidden ...string) {
	t.Helper()
	for name, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		for _, item := range forbidden {
			if bytes.Contains(raw, []byte(item)) {
				t.Fatalf("%s contains forbidden provider data", name)
			}
		}
	}
}

func assertErrorChainExcludes(t *testing.T, appErr error, forbidden ...string) {
	t.Helper()
	for current := appErr; current != nil; current = errors.Unwrap(current) {
		formatted := []byte(fmt.Sprintf("%+v", current))
		for _, item := range forbidden {
			if bytes.Contains(formatted, []byte(item)) {
				t.Fatal("application error chain contains forbidden provider data")
			}
		}
	}
}

func TestSendVerifyCodeRoundsTTLUpToWholeMinutes(t *testing.T) {
	sender := &fakeSmsSender{}
	service, _ := newVerificationSMSService(t, sender)

	appErr := service.SendVerifyCode(context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 5*time.Minute+time.Second)

	if appErr != nil {
		t.Fatalf("SendVerifyCode error=%#v", appErr)
	}
	if !reflect.DeepEqual(sender.input.TemplateParamSet, []string{"654321", "6"}) {
		t.Fatalf("params=%#v", sender.input.TemplateParamSet)
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
		Region: DefaultRegion, Endpoint: DefaultEndpoint, VerifyCodeTTLMinutes: 12, Status: enum.CommonYes, IsDel: enum.CommonNo,
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
	if !reflect.DeepEqual(sender.input.TemplateParamSet, []string{"123456", "12"}) {
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
	repo.config = &Config{ID: 1, SecretIDEnc: secretID, SecretKeyEnc: secretKey, SmsSdkAppID: "1400000000", SignName: "签名", Region: DefaultRegion, Endpoint: DefaultEndpoint, VerifyCodeTTLMinutes: 5, Status: enum.CommonYes, IsDel: enum.CommonNo}
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

func TestConfigUsesVerifyCodeTTLFromConfigRow(t *testing.T) {
	repo := newFakeSmsRepository()
	repo.config = &Config{ID: 1, SmsSdkAppID: "1400000000", SignName: "签名", Region: DefaultRegion, Endpoint: DefaultEndpoint, VerifyCodeTTLMinutes: 14, Status: enum.CommonYes, IsDel: enum.CommonNo}
	service := NewService(repo, secretbox.Box{}, nil)

	result, appErr := service.Config(context.Background())

	if appErr != nil {
		t.Fatalf("Config error = %v", appErr)
	}
	if result.VerifyCodeTTLMinutes != 14 {
		t.Fatalf("ttl = %d, want 14", result.VerifyCodeTTLMinutes)
	}
}

func TestConfigUsesDefaultVerifyCodeTTLWhenConfigMissing(t *testing.T) {
	service := NewService(newFakeSmsRepository(), secretbox.Box{}, nil)

	result, appErr := service.Config(context.Background())

	if appErr != nil {
		t.Fatalf("Config error = %v", appErr)
	}
	if result.Configured || result.VerifyCodeTTLMinutes != 5 {
		t.Fatalf("unexpected default config: %#v", result)
	}
}

func TestSaveConfigPersistsVerifyCodeTTLToConfigRow(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	secretID, _ := box.Encrypt("AKID")
	secretKey, _ := box.Encrypt("SECRET")
	repo := newFakeSmsRepository()
	repo.config = &Config{ID: 1, SecretIDEnc: secretID, SecretKeyEnc: secretKey, SmsSdkAppID: "1400000000", SignName: "旧签名", Region: DefaultRegion, Endpoint: DefaultEndpoint, VerifyCodeTTLMinutes: 5, Status: enum.CommonYes, IsDel: enum.CommonNo}
	service := NewService(repo, box, nil)

	appErr := service.SaveConfig(context.Background(), SaveConfigInput{
		SmsSdkAppID: "1400000000", SignName: "新签名", Region: DefaultRegion, Endpoint: DefaultEndpoint,
		Status: enum.CommonYes, VerifyCodeTTLMinutes: 15,
	})

	if appErr != nil {
		t.Fatalf("SaveConfig error = %v", appErr)
	}
	if repo.config == nil || repo.config.VerifyCodeTTLMinutes != 15 {
		t.Fatalf("saved config = %#v", repo.config)
	}
}

func TestVerifyCodeTTLUsesSmsConfigRow(t *testing.T) {
	repo := newFakeSmsRepository()
	repo.config = &Config{VerifyCodeTTLMinutes: 16}
	service := NewService(repo, secretbox.Box{}, nil)

	got, appErr := service.VerifyCodeTTL(context.Background())

	if appErr != nil || got != 16*time.Minute {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
}

func TestVerifyCodeTTLRejectsMissingSmsConfig(t *testing.T) {
	service := NewService(newFakeSmsRepository(), secretbox.Box{}, nil)

	got, appErr := service.VerifyCodeTTL(context.Background())

	if appErr == nil || appErr.Message != "短信验证码配置未启用" || got != 0 {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
}

func TestVerifyCodeTTLRejectsInvalidSmsConfigRow(t *testing.T) {
	for _, ttl := range []int{0, 61} {
		repo := newFakeSmsRepository()
		repo.config = &Config{VerifyCodeTTLMinutes: ttl}
		service := NewService(repo, secretbox.Box{}, nil)
		got, appErr := service.VerifyCodeTTL(context.Background())
		if appErr == nil || appErr.Message != "验证码有效期必须在 1-60 分钟之间" || got != 0 {
			t.Fatalf("ttl=%d got duration=%s err=%#v", ttl, got, appErr)
		}
	}
}

type fakeSmsSender struct {
	input  SendInput
	result SendResult
	err    error
	onSend func()
	calls  int
}

func (f *fakeSmsSender) Send(ctx context.Context, input SendInput) (SendResult, error) {
	f.calls++
	f.input = input
	if f.onSend != nil {
		f.onSend()
	}
	return f.result, f.err
}

type fakeCodedError struct {
	code    string
	message string
}

func (e fakeCodedError) Error() string {
	if e.message == "" {
		return e.code
	}
	if e.code == "" {
		return e.message
	}
	return e.code + ": " + e.message
}
func (e fakeCodedError) ErrorCode() string { return e.code }

type fakeSmsRepository struct {
	config                 *Config
	templates              map[string]*Template
	logs                   map[uint64]*Log
	createdLogs            []Log
	finishes               map[uint64]LogFinish
	lastTestAt             *time.Time
	lastTestError          string
	nextID                 uint64
	configErr              error
	templateErr            error
	createLogErr           error
	finishLogErr           error
	updateTestResultErr    error
	createLogCalls         int
	finishLogCalls         int
	updateTestResultCalls  int
	createLogCtxErr        error
	finishLogCtxErr        error
	updateTestResultCtxErr error
}

func newFakeSmsRepository() *fakeSmsRepository {
	return &fakeSmsRepository{
		templates: map[string]*Template{},
		logs:      map[uint64]*Log{},
		finishes:  map[uint64]LogFinish{},
		nextID:    1,
	}
}

func (r *fakeSmsRepository) DefaultConfig(ctx context.Context) (*Config, error) {
	return r.config, r.configErr
}
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
	r.updateTestResultCalls++
	r.updateTestResultCtxErr = contextErr(ctx)
	if r.updateTestResultErr != nil {
		return r.updateTestResultErr
	}
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
	return r.templates[scene], r.templateErr
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
	r.createLogCalls++
	r.createLogCtxErr = contextErr(ctx)
	if r.createLogErr != nil {
		return 0, r.createLogErr
	}
	row.ID = r.nextID
	r.nextID++
	copied := row
	r.logs[row.ID] = &copied
	r.createdLogs = append(r.createdLogs, row)
	return row.ID, nil
}
func (r *fakeSmsRepository) FinishLog(ctx context.Context, id uint64, finish LogFinish) error {
	r.finishLogCalls++
	r.finishLogCtxErr = contextErr(ctx)
	if r.finishLogErr != nil {
		return r.finishLogErr
	}
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

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
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
