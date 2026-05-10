package aiconversation

type listRequest struct {
	AgentID  *int64 `form:"agent_id" binding:"omitempty,min=1"`
	BeforeID int64  `form:"before_id" binding:"omitempty,min=1"`
	Limit    int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

type createRequest struct {
	AgentID int64  `json:"agent_id" binding:"required,min=1"`
	Title   string `json:"title" binding:"omitempty,max=100"`
}

type updateRequest struct {
	Title string `json:"title" binding:"required,max=100"`
}
