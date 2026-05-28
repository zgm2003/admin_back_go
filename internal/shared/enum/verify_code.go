package enum

const (
	VerifyCodeSceneLogin          = "login"
	VerifyCodeSceneForget         = "forget"
	VerifyCodeSceneBindPhone      = "bind_phone"
	VerifyCodeSceneBindEmail      = "bind_email"
	VerifyCodeSceneChangePassword = "change_password"
)

var VerifyCodeScenes = []string{
	VerifyCodeSceneLogin,
	VerifyCodeSceneForget,
	VerifyCodeSceneBindPhone,
	VerifyCodeSceneBindEmail,
	VerifyCodeSceneChangePassword,
}

func IsVerifyCodeScene(value string) bool {
	for _, item := range VerifyCodeScenes {
		if value == item {
			return true
		}
	}
	return false
}
