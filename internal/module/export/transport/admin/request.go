package admin

type statusCountRequest struct {
	Kind     string `form:"kind" binding:"omitempty,max=64"`
	Title    string `form:"title" binding:"max=100"`
	FileName string `form:"file_name" binding:"max=255"`
}

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Status      *int   `form:"status" binding:"omitempty"`
	Kind        string `form:"kind" binding:"omitempty,max=64"`
	Title       string `form:"title" binding:"max=100"`
	FileName    string `form:"file_name" binding:"max=255"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
