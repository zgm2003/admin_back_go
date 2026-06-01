package canvas

import "admin_back_go/internal/module/user"

type loginResponse struct {
	Token string             `json:"token"`
	User  *user.InitResponse `json:"user"`
}
