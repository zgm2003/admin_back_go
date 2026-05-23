package appauth

type loginRequest struct {
	Account  string `json:"account" binding:"required,max=100"`
	Password string `json:"password" binding:"required,max=128"`
}

type loginResponse struct {
	Token string  `json:"token"`
	User  appUser `json:"user"`
}

type appUser struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}
