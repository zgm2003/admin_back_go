package dict

import "admin_back_go/internal/enum"

func SmsSceneOptions() []Option[string] {
	return []Option[string]{
		{Label: "手机验证码登录", Value: enum.VerifyCodeSceneLogin},
		{Label: "找回密码", Value: enum.VerifyCodeSceneForget},
		{Label: "绑定/换绑手机", Value: enum.VerifyCodeSceneBindPhone},
		{Label: "验证码改密", Value: enum.VerifyCodeSceneChangePassword},
	}
}

func SmsLogSceneOptions() []Option[string] {
	return append(SmsSceneOptions(), Option[string]{Label: "测试发送", Value: enum.SmsSceneTest})
}

func SmsRegionOptions() []Option[string] {
	return []Option[string]{{Label: "广州", Value: "ap-guangzhou"}}
}

func IsSmsRegion(value string) bool {
	for _, option := range SmsRegionOptions() {
		if option.Value == value {
			return true
		}
	}
	return false
}

func SmsLogStatusOptions() []Option[int] {
	return []Option[int]{
		{Label: "发送中", Value: enum.SmsLogStatusPending},
		{Label: "发送成功", Value: enum.SmsLogStatusSuccess},
		{Label: "发送失败", Value: enum.SmsLogStatusFailed},
	}
}
