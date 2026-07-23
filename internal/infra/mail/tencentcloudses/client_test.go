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
			var safe SendError
			if !errors.As(got, &safe) {
				t.Fatalf("expected SendError, got %T", got)
			}
			if safe.Code != tt.code || safe.ErrorCode() != tt.code || safe.Error() != "邮件服务调用失败" {
				t.Fatalf("unexpected safe provider error: %#v", safe)
			}
			text := got.Error()
			for _, secret := range []string{"AKID-secret", "SECRET-key", "654321", "provider"} {
				if strings.Contains(text, secret) {
					t.Fatalf("raw provider text leaked: %q", text)
				}
			}
			if errors.Unwrap(got) != nil {
				t.Fatal("sanitized provider error must not unwrap a cause")
			}
		})
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

func TestClientSendSanitizesSerializationEmptyResponseAndConstructionErrors(t *testing.T) {
	input := SendInput{SecretID: "AKID-secret", SecretKey: "SECRET-key", Region: "ap-guangzhou", Endpoint: "ses.tencentcloudapi.com", FromEmail: "noreply@example.com", ToEmail: "user@example.com", Subject: "subject", TemplateID: 7, TemplateData: map[string]string{"code": "654321"}}
	tests := []struct {
		name   string
		client *Client
		code   string
	}{
		{name: "serialization", client: &Client{Timeout: time.Second, encodeTemplateData: func(map[string]string) (string, error) { return "", errors.New("marshal AKID-secret 654321") }}, code: "provider_error"},
		{name: "client construction", client: &Client{Timeout: time.Second, newSDKClient: func(SendInput, time.Duration) (sdkClient, error) { return nil, errors.New("construct SECRET-key") }}, code: "provider_error"},
		{name: "empty response", client: &Client{Timeout: time.Second, newSDKClient: func(SendInput, time.Duration) (sdkClient, error) { return &fakeSDKClient{}, nil }}, code: "provider_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.Send(context.Background(), input)
			var safe SendError
			if !errors.As(err, &safe) || safe.Code != tt.code || safe.Error() != "邮件服务调用失败" {
				t.Fatalf("expected sanitized provider error, got %#v", err)
			}
			if strings.Contains(err.Error(), "AKID-secret") || strings.Contains(err.Error(), "SECRET-key") || strings.Contains(err.Error(), "654321") {
				t.Fatalf("sensitive provider error leaked: %v", err)
			}
		})
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

	var safe SendError
	if !errors.As(err, &safe) || safe.Code != "provider_error" || safe.Error() != "邮件服务调用失败" {
		t.Fatalf("nil constructed client must return a safe error, got %#v", err)
	}
}

func stringPtr(value string) *string { return &value }
