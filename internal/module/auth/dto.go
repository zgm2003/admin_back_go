package auth

import "admin_back_go/internal/module/session"

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse = session.TokenResult
