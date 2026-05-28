package tencentcloudsms

import (
	"strings"
	"testing"
	"time"
)

func TestNewUsesDefaultTimeout(t *testing.T) {
	client := New(0)
	if client.Timeout != defaultTimeout {
		t.Fatalf("timeout = %s, want %s", client.Timeout, defaultTimeout)
	}
}

func TestNormalizeInputTrimsWithoutDroppingTemplateParams(t *testing.T) {
	input := normalizeInput(SendInput{
		SecretID: " AKID ", SecretKey: " SECRET ", Region: " ap-guangzhou ", Endpoint: " sms.tencentcloudapi.com ",
		SmsSdkAppID: " 1400000000 ", SignName: " 签名 ", TemplateID: " 12345 ", PhoneNumber: " +8613800138000 ",
		TemplateParamSet: []string{" 123456 ", " 5 "},
	})
	if input.SecretID != "AKID" || input.TemplateParamSet[0] != "123456" || input.TemplateParamSet[1] != "5" {
		t.Fatalf("input not normalized: %#v", input)
	}
}

func TestValidateInputRequiresTencentSendSmsFields(t *testing.T) {
	base := SendInput{SecretID: "AKID", SecretKey: "SECRET", Region: "ap-guangzhou", Endpoint: "sms.tencentcloudapi.com", SmsSdkAppID: "1400000000", SignName: "签名", TemplateID: "12345", PhoneNumber: "+8613800138000"}
	cases := []struct {
		name string
		edit func(*SendInput)
		code string
	}{
		{"secret", func(v *SendInput) { v.SecretID = "" }, "InvalidParameter.SecretId"},
		{"region", func(v *SendInput) { v.Region = "" }, "InvalidParameter.Region"},
		{"app", func(v *SendInput) { v.SmsSdkAppID = "" }, "InvalidParameter.SmsSdkAppId"},
		{"template", func(v *SendInput) { v.TemplateID = "" }, "InvalidParameter.TemplateId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.edit(&input)
			err := validateInput(input)
			if err == nil || !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("validateInput error = %v, want code %s", err, tc.code)
			}
		})
	}
	if err := validateInput(base); err != nil {
		t.Fatalf("validateInput valid input: %v", err)
	}
}

func TestNewKeepsPositiveTimeout(t *testing.T) {
	client := New(3 * time.Second)
	if client.Timeout != 3*time.Second {
		t.Fatalf("timeout = %s", client.Timeout)
	}
}
