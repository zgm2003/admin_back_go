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

	taskPlatforms := NotificationTaskPlatformOptions()
	if len(taskPlatforms) != 3 || taskPlatforms[0].Value != enum.PlatformAll || taskPlatforms[1].Value != enum.PlatformAdmin || taskPlatforms[2].Value != enum.PlatformApp {
		t.Fatalf("unexpected notification task platform options: %#v", taskPlatforms)
	}

	sex := SexOptions()
	if len(sex) != 3 || sex[0].Value != enum.SexUnknown || sex[1].Value != enum.SexMale || sex[2].Value != enum.SexFemale {
		t.Fatalf("unexpected sex options: %#v", sex)
	}

	verifyTypes := UserVerifyTypeOptions()
	if len(verifyTypes) != 2 || verifyTypes[0].Value != enum.VerifyTypePassword || verifyTypes[1].Value != enum.VerifyTypeCode {
		t.Fatalf("unexpected user verify type options: %#v", verifyTypes)
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

	notificationTypes := NotificationTypeOptions()
	if len(notificationTypes) != 4 || notificationTypes[0].Value != enum.NotificationTypeInfo || notificationTypes[3].Value != enum.NotificationTypeError {
		t.Fatalf("unexpected notification type options: %#v", notificationTypes)
	}

	notificationLevels := NotificationLevelOptions()
	if len(notificationLevels) != 2 || notificationLevels[0].Value != enum.NotificationLevelNormal || notificationLevels[1].Value != enum.NotificationLevelUrgent {
		t.Fatalf("unexpected notification level options: %#v", notificationLevels)
	}

	readStatus := NotificationReadStatusOptions()
	if len(readStatus) != 2 || readStatus[0].Value != enum.CommonYes || readStatus[1].Value != enum.CommonNo {
		t.Fatalf("unexpected notification read status options: %#v", readStatus)
	}

	targetTypes := NotificationTargetTypeOptions()
	if len(targetTypes) != 3 || targetTypes[0].Value != enum.NotificationTargetAll || targetTypes[1].Value != enum.NotificationTargetUsers || targetTypes[2].Value != enum.NotificationTargetRoles {
		t.Fatalf("unexpected notification target type options: %#v", targetTypes)
	}

	taskStatuses := NotificationTaskStatusOptions()
	if len(taskStatuses) != 4 || taskStatuses[0].Value != enum.NotificationTaskStatusPending || taskStatuses[3].Value != enum.NotificationTaskStatusFailed {
		t.Fatalf("unexpected notification task status options: %#v", taskStatuses)
	}
}
