package aiconversation

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
	AgentID     *int64 `form:"agent_id" binding:"omitempty,min=1"`
	Title       string `form:"title" binding:"max=100"`
}

type mutationRequest struct {
	AgentID int64  `json:"agent_id" binding:"required,min=1"`
	Title   string `json:"title" binding:"max=100"`
}

type titleRequest struct {
	Title string `json:"title" binding:"required,max=100"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
