package tencentcloudses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

type sdkClient interface {
	SendEmailWithContext(context.Context, *ses.SendEmailRequest) (*ses.SendEmailResponse, error)
}

type sdkClientFactory func(SendInput, time.Duration) (sdkClient, error)
type templateDataEncoder func(map[string]string) (string, error)

type Client struct {
	Timeout            time.Duration
	newSDKClient       sdkClientFactory
	encodeTemplateData templateDataEncoder
}

func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{Timeout: timeout, newSDKClient: defaultSDKClientFactory, encodeTemplateData: TemplateDataJSON}
}

type SendError struct{ Code string }

func (e SendError) Error() string     { return "邮件服务调用失败" }
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
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	encoder := c.encodeTemplateData
	if encoder == nil {
		encoder = TemplateDataJSON
	}
	data, err := encoder(input.TemplateData)
	if err != nil {
		return SendResult{}, wrapSendError(err, input)
	}
	factory := c.newSDKClient
	if factory == nil {
		factory = defaultSDKClientFactory
	}
	client, err := factory(input, timeout)
	if err != nil || client == nil {
		return SendResult{}, wrapSendError(err, input)
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
		return SendResult{}, wrapSendError(err, input)
	}
	if response == nil || response.Response == nil {
		return SendResult{}, wrapSendError(errors.New("tencent ses returned empty response"), input)
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

func defaultSDKClientFactory(input SendInput, timeout time.Duration) (sdkClient, error) {
	return newSDKClient(input, timeout)
}

var providerErrorCodePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,127}$`)

func wrapSendError(err error, input SendInput) error {
	code := "provider_error"
	if sdkErr, ok := err.(*tcerr.TencentCloudSDKError); ok {
		candidate := sdkErr.GetCode()
		if providerErrorCodePattern.MatchString(candidate) && !containsSensitive(candidate, input) {
			code = candidate
		}
	}
	return SendError{Code: code}
}

func containsSensitive(value string, input SendInput) bool {
	for _, secret := range []string{input.SecretID, input.SecretKey} {
		if secret != "" && strings.Contains(value, secret) {
			return true
		}
	}
	for _, templateValue := range input.TemplateData {
		if templateValue != "" && strings.Contains(value, templateValue) {
			return true
		}
	}
	return false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
