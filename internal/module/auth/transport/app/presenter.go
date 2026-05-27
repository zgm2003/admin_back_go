package app

import "admin_back_go/internal/module/user"

type loginResponse struct {
	Token string  `json:"token"`
	User  appUser `json:"user"`
}

type appUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func userFromInit(currentUser *user.InitResponse) appUser {
	if currentUser == nil {
		return appUser{}
	}
	return appUser{ID: currentUser.UserID, Nickname: currentUser.Username, Avatar: currentUser.Avatar}
}
