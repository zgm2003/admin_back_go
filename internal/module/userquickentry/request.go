package userquickentry

type saveRequest struct {
	PermissionIDs []int64 `json:"permission_ids" binding:"required,max=6,dive,min=1"`
}
