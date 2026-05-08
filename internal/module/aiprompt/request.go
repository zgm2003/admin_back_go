package aiprompt

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Title       string `form:"title" binding:"max=100"`
	Category    string `form:"category" binding:"max=50"`
	IsFavorite  *int   `form:"is_favorite" binding:"omitempty,common_status"`
}

type mutationRequest struct {
	Title     string   `json:"title" binding:"required,max=100"`
	Content   string   `json:"content" binding:"required"`
	Category  string   `json:"category" binding:"max=50"`
	Tags      []string `json:"tags"`
	Variables []string `json:"variables"`
}
