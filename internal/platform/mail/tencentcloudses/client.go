package tencentcloudses

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	common "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
)

const defaultTimeout = 10 * time.Second

type SendInput struct {
	SecretID     string
	SecretKey    string
	Region       string
	Endpoint     string
	FromEmail    string
	FromName     string
	ReplyTo      string
	ToEmail      string
	Subject      string
	TemplateID   uint64
	TemplateData map[string]string
}

type SendResult struct {
	RequestID string
	MessageID string
}

type Client struct{ Timeout time.Duration }

func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{Timeout: timeout}
}

type SendError struct {
	Code    string
	Message string
	Cause   error
}

func (e SendError) Error() string {
	if e.Message == "" {
		return e.Cause.Error()
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (e SendError) Unwrap() error     { return e.Cause }
func (e SendError) ErrorCode() string { return e.Code }

func BuildFromEmailAddress(email string, name string) string {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)
	if name == "" {
		return email
	}
	return name + " <" + email + ">"
}

func TemplateDataJSON(values map[string]string) (string, error) {
	if values == nil {
		values = map[string]string{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(values))
	for _, key := range keys {
		ordered[key] = values[key]
	}
	body, err := json.Marshal(ordered)
	if err != nil {
		return "", fmt.Errorf("marshal template data: %w", err)
	}
	return string(body), nil
}

func (c *Client) Send(ctx context.Context, input SendInput) (SendResult, error) {
	if c == nil {
		c = New(defaultTimeout)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	data, err := TemplateDataJSON(input.TemplateData)
	if err != nil {
		return SendResult{}, err
	}
	client, err := newSDKClient(input, c.Timeout)
	if err != nil {
		return SendResult{}, err
	}
	request := ses.NewSendEmailRequest()
	request.FromEmailAddress = common.StringPtr(BuildFromEmailAddress(input.FromEmail, input.FromName))
	request.Destination = common.StringPtrs([]string{input.ToEmail})
	request.Subject = common.StringPtr(input.Subject)
	request.Template = &ses.Template{TemplateID: common.Uint64Ptr(input.TemplateID), TemplateData: common.StringPtr(data)}
	request.TriggerType = common.Uint64Ptr(1)
	if strings.TrimSpace(input.ReplyTo) != "" {
		request.ReplyToAddresses = common.StringPtr(strings.TrimSpace(input.ReplyTo))
	}
	response, err := client.SendEmailWithContext(ctx, request)
	if err != nil {
		return SendResult{}, wrapSendError(err)
	}
	if response == nil || response.Response == nil {
		return SendResult{}, fmt.Errorf("tencent ses returned empty response")
	}
	return SendResult{RequestID: stringValue(response.Response.RequestId), MessageID: stringValue(response.Response.MessageId)}, nil
}

func newSDKClient(input SendInput, timeout time.Duration) (*ses.Client, error) {
	credential := common.NewCredential(input.SecretID, input.SecretKey)
	profile := profile.NewClientProfile()
	profile.HttpProfile.Endpoint = strings.TrimSpace(input.Endpoint)
	profile.HttpProfile.ReqTimeout = int(timeout.Seconds())
	client, err := ses.NewClient(credential, strings.TrimSpace(input.Region), profile)
	if err != nil {
		return nil, fmt.Errorf("create tencent ses client: %w", err)
	}
	return client, nil
}

func wrapSendError(err error) error {
	if sdkErr, ok := err.(*tcerr.TencentCloudSDKError); ok {
		return SendError{Code: sdkErr.GetCode(), Message: sdkErr.GetMessage(), Cause: err}
	}
	return err
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
