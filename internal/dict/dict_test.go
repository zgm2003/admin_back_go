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

	permissionTypes := PermissionTypeOptions()
	if len(permissionTypes) != 3 || permissionTypes[0].Value != enum.PermissionTypeDir || permissionTypes[1].Value != enum.PermissionTypePage || permissionTypes[2].Value != enum.PermissionTypeButton {
		t.Fatalf("unexpected permission type options: %#v", permissionTypes)
	}

	platforms := PlatformOptions()
	if len(platforms) != 2 || platforms[0].Value != enum.PlatformAdmin || platforms[1].Value != enum.PlatformApp {
		t.Fatalf("unexpected platform options: %#v", platforms)
	}

	sex := SexOptions()
	if len(sex) != 3 || sex[0].Value != enum.SexUnknown || sex[1].Value != enum.SexMale || sex[2].Value != enum.SexFemale {
		t.Fatalf("unexpected sex options: %#v", sex)
	}

	levels := LogLevelOptions()
	if len(levels) != 5 || levels[0].Value != enum.LogLevelDebug || levels[4].Value != enum.LogLevelCritical {
		t.Fatalf("unexpected log level options: %#v", levels)
	}

	tails := LogTailOptions()
	if len(tails) != 5 || tails[0].Value != enum.LogTail100 || tails[4].Value != enum.LogTail2000 {
		t.Fatalf("unexpected log tail options: %#v", tails)
	}

	valueTypes := SystemSettingValueTypeOptions()
	if len(valueTypes) != 4 || valueTypes[0].Value != enum.SystemSettingValueString || valueTypes[3].Value != enum.SystemSettingValueJSON {
		t.Fatalf("unexpected system setting value type options: %#v", valueTypes)
	}
}
