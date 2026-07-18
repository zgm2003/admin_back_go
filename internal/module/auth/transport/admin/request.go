package admin

type LoginRequest struct {
	LoginAccount string `json:"login_account" binding:"required,max=100"`
	LoginType    string `json:"login_type" binding:"required,auth_platform_login_type"`
	Password     string `json:"password" binding:"omitempty,max=128"`
	Code         string `json:"code" binding:"omitempty,len=6,numeric"`
}

type SendCodeRequest struct {
	Account       string                `json:"account" binding:"required,max=120"`
	Scene         string                `json:"scene" binding:"required,verify_code_scene"`
	LoginType     string                `json:"login_type" binding:"omitempty,auth_platform_login_type"`
	CaptchaID     string                `json:"captcha_id" binding:"omitempty,max=80"`
	CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer"`
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

type sessionListRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Username    string `form:"username"`
	Platform    string `form:"platform"`
	Status      string `form:"status"`
}

type sessionBatchRevokeRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,max=100,dive,min=1"`
}

type loginLogListRequest struct {
	CurrentPage  int    `form:"current_page"`
	PageSize     int    `form:"page_size"`
	UserID       int64  `form:"user_id"`
	LoginAccount string `form:"login_account"`
	LoginType    string `form:"login_type"`
	IP           string `form:"ip"`
	Platform     string `form:"platform"`
	IsSuccess    *int   `form:"is_success"`
	DateStart    string `form:"date_start"`
	DateEnd      string `form:"date_end"`
}

type captchaAnswerRequest struct {
	X int `json:"x" binding:"min=0,max=10000"`
	Y int `json:"y" binding:"min=0,max=10000"`
}
