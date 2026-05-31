package canvas

import "admin_back_go/internal/module/user"

type loginResponse struct {
	Token string     `json:"token"`
	User  canvasUser `json:"user"`
}

type canvasUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func userFromInit(currentUser *user.InitResponse) canvasUser {
	if currentUser == nil {
		return canvasUser{}
	}
	return canvasUser{ID: currentUser.UserID, Nickname: currentUser.Username, Avatar: currentUser.Avatar}
}
