package app

import (
	"admin_back_go/internal/module/permission"
	usermodule "admin_back_go/internal/module/user"
)

type currentUserResponse struct {
	UserID      int64                  `json:"user_id"`
	Username    string                 `json:"username"`
	Avatar      string                 `json:"avatar"`
	RoleName    string                 `json:"role_name"`
	Permissions []permission.MenuItem  `json:"permissions"`
	Router      []permission.RouteItem `json:"router"`
	ButtonCodes []string               `json:"buttonCodes"`
}

func currentUserFromInit(currentUser *usermodule.InitResponse) currentUserResponse {
	return currentUserResponse{
		UserID:      currentUser.UserID,
		Username:    currentUser.Username,
		Avatar:      currentUser.Avatar,
		RoleName:    currentUser.RoleName,
		Permissions: currentUser.Permissions,
		Router:      currentUser.Router,
		ButtonCodes: currentUser.ButtonCodes,
	}
}
