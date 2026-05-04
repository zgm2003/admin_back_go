package dict

import "admin_back_go/internal/enum"

type Option[T string | int] struct {
	Label string `json:"label"`
	Value T      `json:"value"`
}

func CommonStatusOptions() []Option[int] {
	return []Option[int]{
		{Label: "启用", Value: enum.CommonYes},
		{Label: "禁用", Value: enum.CommonNo},
	}
}

func CommonYesNoOptions() []Option[int] {
	return []Option[int]{
		{Label: "是", Value: enum.CommonYes},
		{Label: "否", Value: enum.CommonNo},
	}
}

func AuthPlatformLoginTypeOptions() []Option[string] {
	return []Option[string]{
		{Label: "邮箱登录", Value: enum.LoginTypeEmail},
		{Label: "手机号登录", Value: enum.LoginTypePhone},
		{Label: "密码登录", Value: enum.LoginTypePassword},
	}
}

func AuthPlatformCaptchaTypeOptions() []Option[string] {
	return []Option[string]{
		{Label: "滑块验证", Value: enum.CaptchaTypeSlide},
	}
}

func PermissionTypeOptions() []Option[int] {
	return []Option[int]{
		{Label: "目录", Value: enum.PermissionTypeDir},
		{Label: "页面", Value: enum.PermissionTypePage},
		{Label: "按钮", Value: enum.PermissionTypeButton},
	}
}

func PlatformOptions() []Option[string] {
	return []Option[string]{
		{Label: enum.PlatformAdmin, Value: enum.PlatformAdmin},
		{Label: enum.PlatformApp, Value: enum.PlatformApp},
	}
}

func SexOptions() []Option[int] {
	return []Option[int]{
		{Label: "未知", Value: enum.SexUnknown},
		{Label: "男", Value: enum.SexMale},
		{Label: "女", Value: enum.SexFemale},
	}
}

func LogLevelOptions() []Option[string] {
	return []Option[string]{
		{Label: enum.LogLevelDebug, Value: enum.LogLevelDebug},
		{Label: enum.LogLevelInfo, Value: enum.LogLevelInfo},
		{Label: enum.LogLevelWarning, Value: enum.LogLevelWarning},
		{Label: enum.LogLevelError, Value: enum.LogLevelError},
		{Label: enum.LogLevelCritical, Value: enum.LogLevelCritical},
	}
}

func LogTailOptions() []Option[int] {
	return []Option[int]{
		{Label: "最近 100 行", Value: enum.LogTail100},
		{Label: "最近 300 行", Value: enum.LogTail300},
		{Label: "最近 500 行", Value: enum.LogTail500},
		{Label: "最近 1000 行", Value: enum.LogTail1000},
		{Label: "最近 2000 行", Value: enum.LogTail2000},
	}
}

func SystemSettingValueTypeOptions() []Option[int] {
	return []Option[int]{
		{Label: "字符串", Value: enum.SystemSettingValueString},
		{Label: "数字", Value: enum.SystemSettingValueNumber},
		{Label: "布尔", Value: enum.SystemSettingValueBool},
		{Label: "JSON", Value: enum.SystemSettingValueJSON},
	}
}
