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

type LoginRequest struct {
	LoginAccount  string          `json:"login_account"`
	LoginType     string          `json:"login_type"`
	Password      string          `json:"password"`
	Code          string          `json:"code"`
	CaptchaID     string          `json:"captcha_id"`
	CaptchaAnswer *captcha.Answer `json:"captcha_answer"`
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

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse = session.TokenResult
