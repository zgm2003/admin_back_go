package clientversion

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Platform    string `form:"platform" binding:"omitempty,client_platform"`
}

type saveRequest struct {
	Version     string `json:"version" binding:"required,max=20"`
	Notes       string `json:"notes" binding:"max=1000"`
	FileURL     string `json:"file_url" binding:"required,url,max=500"`
	Signature   string `json:"signature" binding:"required"`
	Platform    string `json:"platform" binding:"required,client_platform"`
	FileSize    int64  `json:"file_size" binding:"omitempty,min=0"`
	ForceUpdate int    `json:"force_update" binding:"omitempty,common_yes_no"`
}

type forceUpdateRequest struct {
	ForceUpdate int `json:"force_update" binding:"required,common_yes_no"`
}

type currentCheckRequest struct {
	Version  string `form:"version" binding:"required,max=20"`
	Platform string `form:"platform" binding:"omitempty,client_platform"`
}

type updateJSONRequest struct {
	Platform string `form:"platform" binding:"omitempty,client_platform"`
}
