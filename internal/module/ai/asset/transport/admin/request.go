package admin

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Keyword     string `form:"keyword" binding:"omitempty,max=128"`
	Type        string `form:"type" binding:"omitempty,oneof=text image video"`
	Status      int    `form:"status" binding:"omitempty,oneof=1 2"`
}

type assetRequest struct {
	Slug        string `json:"slug" binding:"required,max=191"`
	Type        string `json:"type" binding:"required,oneof=text image video"`
	Category    string `json:"category" binding:"omitempty,max=191"`
	Title       string `json:"title" binding:"required,max=191"`
	CoverURL    string `json:"cover_url" binding:"omitempty,max=1024"`
	Description string `json:"description" binding:"omitempty,max=512"`
	Content     string `json:"content" binding:"omitempty,max=20000"`
	URL         string `json:"url" binding:"omitempty,max=1024"`
	TagsJSON    string `json:"tags_json" binding:"omitempty,max=4000"`
	Status      int    `json:"status" binding:"omitempty,oneof=0 1 2"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
