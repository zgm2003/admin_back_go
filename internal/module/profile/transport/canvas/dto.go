package canvas

import "admin_back_go/internal/module/profile"

type canvasUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func canvasUserFromInit(currentUser *profile.InitResponse) canvasUser {
	if currentUser == nil {
		return canvasUser{}
	}
	return canvasUser{ID: currentUser.UserID, Nickname: currentUser.Username, Avatar: currentUser.Avatar}
}
