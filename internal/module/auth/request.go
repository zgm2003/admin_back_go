package auth

type LoginRequest struct {
	LoginAccount  string                `json:"login_account" binding:"required,max=100"`
	LoginType     string                `json:"login_type" binding:"required,auth_platform_login_type"`
	Password      string                `json:"password" binding:"required,max=128"`
	Code          string                `json:"code"`
	CaptchaID     string                `json:"captcha_id" binding:"required,max=80"`
	CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type captchaAnswerRequest struct {
	X int `json:"x" binding:"min=0,max=10000"`
	Y int `json:"y" binding:"min=0,max=10000"`
}
