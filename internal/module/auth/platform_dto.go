package auth

import "admin_back_go/internal/module/user"

type platformLoginRequest struct {
	LoginType     string                `json:"login_type" binding:"required,auth_platform_login_type"`
	LoginAccount  string                `json:"login_account" binding:"required,max=100"`
	Password      string                `json:"password" binding:"omitempty,max=128"`
	Code          string                `json:"code" binding:"omitempty,max=20"`
	CaptchaID     string                `json:"captcha_id" binding:"omitempty,max=128"`
	CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer"`
}

type platformLoginResponse struct {
	Token string       `json:"token"`
	User  platformUser `json:"user"`
}

type platformUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func platformUserFromInit(currentUser *user.InitResponse) platformUser {
	if currentUser == nil {
		return platformUser{}
	}
	return platformUser{ID: currentUser.UserID, Nickname: currentUser.Username, Avatar: currentUser.Avatar}
}
