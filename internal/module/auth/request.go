package auth

type LoginRequest struct {
	LoginAccount  string                `json:"login_account" binding:"required,max=100"`
	LoginType     string                `json:"login_type" binding:"required,auth_platform_login_type"`
	Password      string                `json:"password" binding:"omitempty,max=128"`
	Code          string                `json:"code" binding:"omitempty,len=6,numeric"`
	CaptchaID     string                `json:"captcha_id" binding:"omitempty,max=80"`
	CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer"`
}

type SendCodeRequest struct {
	Account string `json:"account" binding:"required,max=120"`
	Scene   string `json:"scene" binding:"required,verify_code_scene"`
}

type ForgetPasswordRequest struct {
	Account         string `json:"account" binding:"required,max=120"`
	Code            string `json:"code" binding:"required,len=6,numeric"`
	NewPassword     string `json:"new_password" binding:"required,min=6,max=128"`
	ConfirmPassword string `json:"confirm_password" binding:"required,min=6,max=128"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type captchaAnswerRequest struct {
	X int `json:"x" binding:"min=0,max=10000"`
	Y int `json:"y" binding:"min=0,max=10000"`
}
