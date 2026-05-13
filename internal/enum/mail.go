package enum

const (
	MailLogStatusPending = 1
	MailLogStatusSuccess = 2
	MailLogStatusFailed  = 3

	MailSceneTest = "test"
)

func IsMailLogStatus(value int) bool {
	return value == MailLogStatusPending || value == MailLogStatusSuccess || value == MailLogStatusFailed
}

func IsMailTemplateScene(value string) bool {
	switch value {
	case VerifyCodeSceneLogin, VerifyCodeSceneForget, VerifyCodeSceneBindEmail, VerifyCodeSceneChangePassword:
		return true
	default:
		return false
	}
}

func IsMailLogScene(value string) bool {
	return IsMailTemplateScene(value) || value == MailSceneTest
}
