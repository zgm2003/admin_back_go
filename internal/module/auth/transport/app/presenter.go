package app

import (
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/user"
)

type loginResponse struct {
	Token string    `json:"token"`
	User  loginUser `json:"user"`
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
