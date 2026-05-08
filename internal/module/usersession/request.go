package usersession

type listRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Username    string `form:"username"`
	Platform    string `form:"platform"`
	Status      string `form:"status"`
}

type batchRevokeRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,max=100,dive,min=1"`
}
