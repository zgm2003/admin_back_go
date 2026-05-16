package tencentcloudsms

import (
	"context"
	"fmt"
	"strings"
	"time"

	common "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

const defaultTimeout = 10 * time.Second

type SendInput struct {
	SecretID         string
	SecretKey        string
	Region           string
	Endpoint         string
	SmsSdkAppID      string
	SignName         string
	TemplateID       string
	PhoneNumber      string
	TemplateParamSet []string
}

type SendResult struct {
	RequestID string
	SerialNo  string
	Fee       uint64
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
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "tencent sms send failed"
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (e SendError) Unwrap() error     { return e.Cause }
func (e SendError) ErrorCode() string { return e.Code }

func (c *Client) Send(ctx context.Context, input SendInput) (SendResult, error) {
	if c == nil {
		c = New(defaultTimeout)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return SendResult{}, err
	}
	client, err := newSDKClient(input, c.Timeout)
	if err != nil {
		return SendResult{}, err
	}
	request := sms.NewSendSmsRequest()
	request.PhoneNumberSet = common.StringPtrs([]string{input.PhoneNumber})
	request.SmsSdkAppId = common.StringPtr(input.SmsSdkAppID)
	request.SignName = common.StringPtr(input.SignName)
	request.TemplateId = common.StringPtr(input.TemplateID)
	request.TemplateParamSet = common.StringPtrs(input.TemplateParamSet)

	response, err := client.SendSmsWithContext(ctx, request)
	if err != nil {
		return SendResult{}, wrapSendError(err)
	}
	if response == nil || response.Response == nil {
		return SendResult{}, fmt.Errorf("tencent sms returned empty response")
	}
	result := SendResult{RequestID: stringValue(response.Response.RequestId)}
	if len(response.Response.SendStatusSet) == 0 || response.Response.SendStatusSet[0] == nil {
		return result, fmt.Errorf("tencent sms returned empty send status")
	}
	status := response.Response.SendStatusSet[0]
	result.SerialNo = stringValue(status.SerialNo)
	result.Fee = uint64Value(status.Fee)
	code := stringValue(status.Code)
	if !strings.EqualFold(code, "Ok") {
		return result, SendError{Code: code, Message: stringValue(status.Message)}
	}
	return result, nil
}

func normalizeInput(input SendInput) SendInput {
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.Region = strings.TrimSpace(input.Region)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.SmsSdkAppID = strings.TrimSpace(input.SmsSdkAppID)
	input.SignName = strings.TrimSpace(input.SignName)
	input.TemplateID = strings.TrimSpace(input.TemplateID)
	input.PhoneNumber = strings.TrimSpace(input.PhoneNumber)
	for i := range input.TemplateParamSet {
		input.TemplateParamSet[i] = strings.TrimSpace(input.TemplateParamSet[i])
	}
	return input
}

func validateInput(input SendInput) error {
	switch {
	case input.SecretID == "":
		return SendError{Code: "InvalidParameter.SecretId", Message: "SecretId is required"}
	case input.SecretKey == "":
		return SendError{Code: "InvalidParameter.SecretKey", Message: "SecretKey is required"}
	case input.Region == "":
		return SendError{Code: "InvalidParameter.Region", Message: "Region is required"}
	case input.Endpoint == "":
		return SendError{Code: "InvalidParameter.Endpoint", Message: "Endpoint is required"}
	case input.SmsSdkAppID == "":
		return SendError{Code: "InvalidParameter.SmsSdkAppId", Message: "SmsSdkAppId is required"}
	case input.SignName == "":
		return SendError{Code: "InvalidParameter.SignName", Message: "SignName is required"}
	case input.TemplateID == "":
		return SendError{Code: "InvalidParameter.TemplateId", Message: "TemplateId is required"}
	case input.PhoneNumber == "":
		return SendError{Code: "InvalidParameter.PhoneNumberSet", Message: "phone number is required"}
	default:
		return nil
	}
}

func newSDKClient(input SendInput, timeout time.Duration) (*sms.Client, error) {
	credential := common.NewCredential(input.SecretID, input.SecretKey)
	profile := profile.NewClientProfile()
	profile.HttpProfile.Endpoint = input.Endpoint
	profile.HttpProfile.ReqTimeout = int(timeout.Seconds())
	client, err := sms.NewClient(credential, input.Region, profile)
	if err != nil {
		return nil, fmt.Errorf("create tencent sms client: %w", err)
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

func uint64Value(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}
