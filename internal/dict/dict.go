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

func NotificationTaskPlatformOptions() []Option[string] {
	return []Option[string]{
		{Label: "全平台", Value: enum.PlatformAll},
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

func UserVerifyTypeOptions() []Option[string] {
	return []Option[string]{
		{Label: "密码验证", Value: enum.VerifyTypePassword},
		{Label: "验证码验证", Value: enum.VerifyTypeCode},
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

func NotificationTypeOptions() []Option[int] {
	return []Option[int]{
		{Label: enum.NotificationTypeLabels[enum.NotificationTypeInfo], Value: enum.NotificationTypeInfo},
		{Label: enum.NotificationTypeLabels[enum.NotificationTypeSuccess], Value: enum.NotificationTypeSuccess},
		{Label: enum.NotificationTypeLabels[enum.NotificationTypeWarning], Value: enum.NotificationTypeWarning},
		{Label: enum.NotificationTypeLabels[enum.NotificationTypeError], Value: enum.NotificationTypeError},
	}
}

func NotificationLevelOptions() []Option[int] {
	return []Option[int]{
		{Label: enum.NotificationLevelLabels[enum.NotificationLevelNormal], Value: enum.NotificationLevelNormal},
		{Label: enum.NotificationLevelLabels[enum.NotificationLevelUrgent], Value: enum.NotificationLevelUrgent},
	}
}

func NotificationReadStatusOptions() []Option[int] {
	return []Option[int]{
		{Label: "已读", Value: enum.CommonYes},
		{Label: "未读", Value: enum.CommonNo},
	}
}

func NotificationTargetTypeOptions() []Option[int] {
	return []Option[int]{
		{Label: enum.NotificationTargetTypeLabels[enum.NotificationTargetAll], Value: enum.NotificationTargetAll},
		{Label: enum.NotificationTargetTypeLabels[enum.NotificationTargetUsers], Value: enum.NotificationTargetUsers},
		{Label: enum.NotificationTargetTypeLabels[enum.NotificationTargetRoles], Value: enum.NotificationTargetRoles},
	}
}

func NotificationTaskStatusOptions() []Option[int] {
	return []Option[int]{
		{Label: enum.NotificationTaskStatusLabels[enum.NotificationTaskStatusPending], Value: enum.NotificationTaskStatusPending},
		{Label: enum.NotificationTaskStatusLabels[enum.NotificationTaskStatusSending], Value: enum.NotificationTaskStatusSending},
		{Label: enum.NotificationTaskStatusLabels[enum.NotificationTaskStatusSuccess], Value: enum.NotificationTaskStatusSuccess},
		{Label: enum.NotificationTaskStatusLabels[enum.NotificationTaskStatusFailed], Value: enum.NotificationTaskStatusFailed},
	}
}

func PayChannelOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.PayChannels))
	for _, value := range enum.PayChannels {
		options = append(options, Option[int]{
			Label: enum.PayChannelLabels[value],
			Value: value,
		})
	}
	return options
}

func PayMethodOptions() []Option[string] {
	return payMethodOptions(enum.PayMethods)
}

func PayMethodOptionsForChannel(channel int) []Option[string] {
	return payMethodOptions(enum.PayDefaultSupportedMethods(channel))
}

func PayTxnStatusOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.PayTxnStatuses))
	for _, value := range enum.PayTxnStatuses {
		options = append(options, Option[int]{
			Label: enum.PayTxnStatusLabels[value],
			Value: value,
		})
	}
	return options
}

func PayOrderTypeOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.PayOrderTypes))
	for _, value := range enum.PayOrderTypes {
		options = append(options, Option[int]{
			Label: enum.PayOrderTypeLabels[value],
			Value: value,
		})
	}
	return options
}

func PayStatusOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.PayStatuses))
	for _, value := range enum.PayStatuses {
		options = append(options, Option[int]{
			Label: enum.PayStatusLabels[value],
			Value: value,
		})
	}
	return options
}

func PayBizStatusOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.PayBizStatuses))
	for _, value := range enum.PayBizStatuses {
		options = append(options, Option[int]{
			Label: enum.PayBizStatusLabels[value],
			Value: value,
		})
	}
	return options
}

func RechargePresetOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.RechargePresets))
	for _, item := range enum.RechargePresets {
		options = append(options, Option[int]{
			Label: item.Label,
			Value: item.Value,
		})
	}
	return options
}

func WalletTypeOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.WalletTypes))
	for _, value := range enum.WalletTypes {
		options = append(options, Option[int]{
			Label: enum.WalletTypeLabels[value],
			Value: value,
		})
	}
	return options
}

func WalletSourceOptions() []Option[int] {
	options := make([]Option[int], 0, len(enum.WalletSources))
	for _, value := range enum.WalletSources {
		options = append(options, Option[int]{
			Label: enum.WalletSourceLabels[value],
			Value: value,
		})
	}
	return options
}

func UploadDriverOptions() []Option[string] {
	options := make([]Option[string], 0, len(enum.UploadDrivers))
	for _, value := range enum.UploadDrivers {
		options = append(options, Option[string]{
			Label: enum.UploadDriverLabels[value],
			Value: value,
		})
	}
	return options
}

func UploadImageExtOptions() []Option[string] {
	return uploadExtOptions(enum.UploadImageExts)
}

func UploadFileExtOptions() []Option[string] {
	return uploadExtOptions(enum.UploadFileExts)
}

func payMethodOptions(values []string) []Option[string] {
	options := make([]Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, Option[string]{
			Label: enum.PayMethodLabels[value],
			Value: value,
		})
	}
	return options
}

func uploadExtOptions(values []string) []Option[string] {
	options := make([]Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, Option[string]{
			Label: value,
			Value: value,
		})
	}
	return options
}
