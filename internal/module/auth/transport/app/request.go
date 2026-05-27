package app

type SendCodeRequest struct {
	Account string `json:"account" binding:"required,max=120"`
	Scene   string `json:"scene" binding:"required,verify_code_scene"`
}

type captchaAnswerRequest struct {
	X int `json:"x" binding:"min=0,max=10000"`
	Y int `json:"y" binding:"min=0,max=10000"`
}

type LoginRequest struct {
	LoginType     string                `json:"login_type" binding:"required,auth_platform_login_type"`
	LoginAccount  string                `json:"login_account" binding:"required,max=100"`
	Password      string                `json:"password" binding:"omitempty,max=128"`
	Code          string                `json:"code" binding:"omitempty,max=20"`
	CaptchaID     string                `json:"captcha_id" binding:"omitempty,max=128"`
	CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer"`
}
