package role

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Name        string `form:"name" binding:"omitempty,max=64"`
}

type mutationRequest struct {
	Name          string  `json:"name" binding:"required,max=64"`
	PermissionIDs []int64 `json:"permission_id" binding:"omitempty,dive,gt=0"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
