package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestCoreDictOptionsComeFromEnums(t *testing.T) {
	status := CommonStatusOptions()
	if len(status) != 2 || status[0].Value != enum.CommonYes || status[0].Label != "启用" || status[1].Value != enum.CommonNo || status[1].Label != "禁用" {
		t.Fatalf("unexpected common status options: %#v", status)
	}

	loginTypes := AuthPlatformLoginTypeOptions()
	if len(loginTypes) != 3 || loginTypes[0].Value != enum.LoginTypeEmail || loginTypes[1].Value != enum.LoginTypePhone || loginTypes[2].Value != enum.LoginTypePassword {
		t.Fatalf("unexpected auth platform login type options: %#v", loginTypes)
	}

	captchaTypes := AuthPlatformCaptchaTypeOptions()
	if len(captchaTypes) != 1 || captchaTypes[0].Value != enum.CaptchaTypeSlide || captchaTypes[0].Label != "滑块验证" {
		t.Fatalf("unexpected auth platform captcha type options: %#v", captchaTypes)
	}
}
