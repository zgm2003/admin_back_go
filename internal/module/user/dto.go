package user

import "admin_back_go/internal/module/permission"

type InitInput struct {
	UserID   int64
	Platform string
}

type InitResponse struct {
	UserID      int64                  `json:"user_id"`
	Username    string                 `json:"username"`
	Avatar      string                 `json:"avatar"`
	RoleName    string                 `json:"role_name"`
	Permissions []permission.MenuItem  `json:"permissions"`
	Router      []permission.RouteItem `json:"router"`
	ButtonCodes []string               `json:"buttonCodes"`
	QuickEntry  []QuickEntry           `json:"quick_entry"`
}
