package tencentcloudses

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tcerr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	ses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
)

func TestBuildFromEmailAddress(t *testing.T) {
	if got := BuildFromEmailAddress("noreply@example.com", "Admin"); got != "Admin <noreply@example.com>" {
		t.Fatalf("unexpected from address: %q", got)
	}
	if got := BuildFromEmailAddress("noreply@example.com", ""); got != "noreply@example.com" {
		t.Fatalf("unexpected bare from address: %q", got)
	}
}

func TestTemplateDataJSONIsStable(t *testing.T) {
	got, err := TemplateDataJSON(map[string]string{"ttl_minutes": "5", "code": "123456"})
	if err != nil {
		t.Fatalf("TemplateDataJSON returned error: %v", err)
	}
	if got != `{"code":"123456","ttl_minutes":"5"}` {
		t.Fatalf("unexpected template data: %s", got)
	}
}

func TestWrapSendErrorSanitizesAllNonDirectOrSensitiveProviderErrors(t *testing.T) {
	input := SendInput{SecretID: "AKID-secret", SecretKey: "SECRET-key", TemplateData: map[string]string{"code": "654321", "ttl_minutes": "5"}}
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "safe direct typed", err: tcerr.NewTencentCloudSDKError("FailedOperation.TemplateNotApproved", "AKID-secret SECRET-key 654321", "req"), code: "FailedOperation.TemplateNotApproved"},
		{name: "secret in code", err: tcerr.NewTencentCloudSDKError("AKID-secret", "provider", "req"), code: "provider_error"},
		{name: "malformed code", err: tcerr.NewTencentCloudSDKError("9 bad", "provider", "req"), code: "provider_error"},
		{name: "wrapped typed", err: wrappedProviderError{err: tcerr.NewTencentCloudSDKError("FailedOperation.TemplateNotApproved", "provider", "req")}, code: "provider_error"},
		{name: "unknown", err: errors.New("provider AKID-secret SECRET-key 654321"), code: "provider_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapSendError(tt.err, input)
			assertSanitizedSendError(t, got, tt.code, "AKID-secret", "SECRET-key", "654321", "provider")
		})
	}
}

func assertSanitizedSendError(t *testing.T, err error, wantCode string, sensitive ...string) {
	t.Helper()
	safe, ok := err.(SendError)
	if !ok {
		t.Fatalf("expected direct SendError, got %T", err)
	}
	if safe.Code != wantCode || safe.ErrorCode() != wantCode || err.Error() != "邮件服务调用失败" {
		t.Fatalf("unexpected safe provider error: %#v", safe)
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("sanitized provider error must not unwrap a cause")
	}
	if _, ok := err.(interface{ Cause() error }); ok {
		t.Fatal("sanitized provider error must not expose Cause")
	}
	for _, value := range sensitive {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("sensitive provider text leaked %q: %q", value, err.Error())
		}
	}
}

type wrappedProviderError struct{ err error }

func (e wrappedProviderError) Error() string { return "wrapped: " + e.err.Error() }
func (e wrappedProviderError) Unwrap() error { return e.err }

type fakeSDKClient struct {
	ctx      context.Context
	request  *ses.SendEmailRequest
	response *ses.SendEmailResponse
	err      error
}

func (f *fakeSDKClient) SendEmailWithContext(ctx context.Context, request *ses.SendEmailRequest) (*ses.SendEmailResponse, error) {
	f.ctx = ctx
	f.request = request
	return f.response, f.err
}

func TestClientSendSanitizesSerializationError(t *testing.T) {
	input := sensitiveSendInput()
	client := &Client{Timeout: time.Second, encodeTemplateData: func(map[string]string) (string, error) {
		return "", errors.New("marshal AKID-secret SECRET-key 654321")
	}}

	_, err := client.Send(context.Background(), input)

	assertSanitizedSendError(t, err, "provider_error", "marshal", input.SecretID, input.SecretKey, input.TemplateData["code"])
}

func TestClientSendSanitizesClientConstructionError(t *testing.T) {
	input := sensitiveSendInput()
	client := &Client{Timeout: time.Second, newSDKClient: func(SendInput, time.Duration) (sdkClient, error) {
		return nil, errors.New("construct AKID-secret SECRET-key 654321")
	}}

	_, err := client.Send(context.Background(), input)

	assertSanitizedSendError(t, err, "provider_error", "construct", input.SecretID, input.SecretKey, input.TemplateData["code"])
}

func TestClientSendSanitizesEmptyResponseErrorContainingSensitiveValue(t *testing.T) {
	input := sensitiveSendInput()
	input.TemplateData["code"] = "tencent ses returned empty response"
	client := &Client{Timeout: time.Second, newSDKClient: func(SendInput, time.Duration) (sdkClient, error) {
		return &fakeSDKClient{}, nil
	}}

	_, err := client.Send(context.Background(), input)

	assertSanitizedSendError(t, err, "provider_error", input.SecretID, input.SecretKey, input.TemplateData["code"])
}

func sensitiveSendInput() SendInput {
	return SendInput{
		SecretID: "AKID-secret", SecretKey: "SECRET-key", Region: "ap-guangzhou", Endpoint: "ses.tencentcloudapi.com",
		FromEmail: "noreply@example.com", ToEmail: "user@example.com", Subject: "subject", TemplateID: 7,
		TemplateData: map[string]string{"code": "654321"},
	}
}

func TestClientSendPreservesEarlierIncomingDeadline(t *testing.T) {
	now := time.Now()
	incomingDeadline := now.Add(100 * time.Millisecond)
	fake := &fakeSDKClient{response: &ses.SendEmailResponse{Response: &ses.SendEmailResponseParams{RequestId: stringPtr("req"), MessageId: stringPtr("msg")}}}
	client := &Client{
		Timeout:      time.Hour,
		newSDKClient: func(SendInput, time.Duration) (sdkClient, error) { return fake, nil },
	}
	ctx, cancel := context.WithDeadline(context.Background(), incomingDeadline)
	defer cancel()

	result, err := client.Send(ctx, SendInput{SecretID: "id", SecretKey: "key", Region: "region", Endpoint: "endpoint", FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "subject", TemplateID: 1, TemplateData: map[string]string{"code": "123456"}})

	if err != nil || result.RequestID != "req" || result.MessageID != "msg" {
		t.Fatalf("unexpected send result: %#v %v", result, err)
	}
	deadline, ok := fake.ctx.Deadline()
	if !ok || !deadline.Equal(incomingDeadline) {
		t.Fatalf("client must not extend incoming deadline, got %v", deadline)
	}
}

func TestClientSendSanitizesNilConstructedClient(t *testing.T) {
	client := &Client{
		Timeout:      time.Second,
		newSDKClient: func(SendInput, time.Duration) (sdkClient, error) { return nil, nil },
	}

	_, err := client.Send(context.Background(), SendInput{TemplateData: map[string]string{"code": "654321"}})

	assertSanitizedSendError(t, err, "provider_error", "654321")
}

func stringPtr(value string) *string { return &value }
