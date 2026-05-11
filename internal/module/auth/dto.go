package auth

import (
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"
)

type LoginTypeOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type LoginConfigResponse struct {
	LoginTypeArr   []LoginTypeOption `json:"login_type_arr"`
	CaptchaEnabled bool              `json:"captcha_enabled"`
	CaptchaType    string            `json:"captcha_type"`
}

type LoginInput struct {
	LoginAccount  string
	LoginType     string
	Password      string
	Code          string
	CaptchaID     string
	CaptchaAnswer *captcha.Answer
	Platform      string
	DeviceID      string
	ClientIP      string
	UserAgent     string
}

type SendCodeInput struct {
	Account string
	Scene   string
}

type ForgetPasswordInput struct {
	Account         string
	Code            string
	NewPassword     string
	ConfirmPassword string
}

type LoginResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	IsNewUser        bool   `json:"is_new_user"`
}

type RefreshResponse = session.TokenResult

func loginResponseFromToken(result *session.TokenResult, isNewUser bool) *LoginResponse {
	if result == nil {
		return nil
	}
	return &LoginResponse{
		AccessToken:      result.AccessToken,
		RefreshToken:     result.RefreshToken,
		ExpiresIn:        result.ExpiresIn,
		RefreshExpiresIn: result.RefreshExpiresIn,
		IsNewUser:        isNewUser,
	}
}
