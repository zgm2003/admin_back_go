package enum

const (
	SmsLogStatusPending = 1
	SmsLogStatusSuccess = 2
	SmsLogStatusFailed  = 3

	SmsSceneTest = "test"
)

func IsSmsLogStatus(value int) bool {
	return value == SmsLogStatusPending || value == SmsLogStatusSuccess || value == SmsLogStatusFailed
}

func IsSmsTemplateScene(value string) bool {
	switch value {
	case VerifyCodeSceneLogin, VerifyCodeSceneForget, VerifyCodeSceneBindPhone, VerifyCodeSceneChangePassword:
		return true
	default:
		return false
	}
}

func IsSmsLogScene(value string) bool {
	return IsSmsTemplateScene(value) || value == SmsSceneTest
}
