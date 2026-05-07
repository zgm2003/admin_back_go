package usersession

type listRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Username    string `form:"username"`
	Platform    string `form:"platform"`
	Status      string `form:"status"`
}
