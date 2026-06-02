package canvas

import (
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/user"
)

type loginResponse struct {
	tokenResponse
	User loginUser `json:"user"`
}

type tokenResponse struct {
	Token            string `json:"token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

type loginUser struct {
	UserID      int64                  `json:"user_id"`
	Username    string                 `json:"username"`
	Avatar      string                 `json:"avatar"`
	RoleName    string                 `json:"role_name"`
	Permissions []permission.MenuItem  `json:"permissions"`
	Router      []permission.RouteItem `json:"router"`
	ButtonCodes []string               `json:"buttonCodes"`
}

func userFromInit(currentUser *user.InitResponse) loginUser {
	return loginUser{
		UserID:      currentUser.UserID,
		Username:    currentUser.Username,
		Avatar:      currentUser.Avatar,
		RoleName:    currentUser.RoleName,
		Permissions: currentUser.Permissions,
		Router:      currentUser.Router,
		ButtonCodes: currentUser.ButtonCodes,
	}
}

func tokenResponseFromResult(result *authmodule.TokenResult) tokenResponse {
	if result == nil {
		return tokenResponse{}
	}
	return tokenResponse{
		Token:            result.AccessToken,
		RefreshToken:     result.RefreshToken,
		ExpiresIn:        result.ExpiresIn,
		RefreshExpiresIn: result.RefreshExpiresIn,
	}
}

func tokenResponseFromLogin(result *authmodule.LoginResponse) tokenResponse {
	if result == nil {
		return tokenResponse{}
	}
	return tokenResponse{
		Token:            result.AccessToken,
		RefreshToken:     result.RefreshToken,
		ExpiresIn:        result.ExpiresIn,
		RefreshExpiresIn: result.RefreshExpiresIn,
	}
}
