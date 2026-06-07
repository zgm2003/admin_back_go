package admin

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Keyword     string `form:"keyword" binding:"omitempty,max=128"`
	Category    string `form:"category" binding:"omitempty,max=191"`
	Status      int    `form:"status" binding:"omitempty,oneof=1 2"`
}

type promptRequest struct {
	Slug      string `json:"slug" binding:"required,max=191"`
	Category  string `json:"category" binding:"omitempty,max=191"`
	Title     string `json:"title" binding:"required,max=191"`
	CoverURL  string `json:"cover_url" binding:"omitempty,max=1024"`
	Prompt    string `json:"prompt" binding:"required,max=20000"`
	Preview   string `json:"preview" binding:"omitempty,max=512"`
	TagsJSON  string `json:"tags_json" binding:"omitempty,max=4000"`
	SourceURL string `json:"source_url" binding:"omitempty,max=1024"`
	Status    int    `json:"status" binding:"omitempty,oneof=0 1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}
