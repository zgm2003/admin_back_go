package permission

type permissionListRequest struct {
	Platform string `form:"platform" binding:"required,platform_scope"`
	Name     string `form:"name" binding:"omitempty,max=100"`
	Path     string `form:"path" binding:"omitempty,max=255"`
	Type     int    `form:"type" binding:"omitempty,permission_type"`
}

type permissionMutationRequest struct {
	Platform  string `json:"platform" binding:"required,platform_scope"`
	Type      int    `json:"type" binding:"required,permission_type"`
	Name      string `json:"name" binding:"required,max=100"`
	ParentID  int64  `json:"parent_id" binding:"min=0"`
	Icon      string `json:"icon" binding:"omitempty,max=100"`
	Path      string `json:"path" binding:"omitempty,max=255"`
	Component string `json:"component" binding:"omitempty,max=255"`
	I18nKey   string `json:"i18n_key" binding:"omitempty,max=100"`
	Code      string `json:"code" binding:"omitempty,max=100"`
	Sort      int    `json:"sort" binding:"required,min=1,max=1000"`
	ShowMenu  int    `json:"show_menu" binding:"omitempty,common_yes_no"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
