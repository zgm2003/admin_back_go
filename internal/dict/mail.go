package dict

import "admin_back_go/internal/enum"

func MailSceneOptions() []Option[string] {
	return []Option[string]{
		{Label: "邮箱验证码登录", Value: enum.VerifyCodeSceneLogin},
		{Label: "找回密码", Value: enum.VerifyCodeSceneForget},
		{Label: "绑定/换绑邮箱", Value: enum.VerifyCodeSceneBindEmail},
		{Label: "验证码改密", Value: enum.VerifyCodeSceneChangePassword},
	}
}

func MailLogSceneOptions() []Option[string] {
	return append(MailSceneOptions(), Option[string]{Label: "测试发送", Value: enum.MailSceneTest})
}

func MailRegionOptions() []Option[string] {
	return []Option[string]{
		{Label: "广州", Value: "ap-guangzhou"},
		{Label: "香港", Value: "ap-hongkong"},
	}
}

func IsMailRegion(value string) bool {
	for _, option := range MailRegionOptions() {
		if option.Value == value {
			return true
		}
	}
	return false
}

func MailLogStatusOptions() []Option[int] {
	return []Option[int]{
		{Label: "发送中", Value: enum.MailLogStatusPending},
		{Label: "发送成功", Value: enum.MailLogStatusSuccess},
		{Label: "发送失败", Value: enum.MailLogStatusFailed},
	}
}
