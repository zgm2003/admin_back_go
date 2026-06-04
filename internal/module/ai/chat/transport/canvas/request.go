package canvas

type chatCompletionRequest struct {
	AgentID int64  `json:"agent_id" binding:"required,gt=0"`
	ModelID string `json:"model" binding:"omitempty,max=128"`
	Message string `json:"message" binding:"required,max=20000"`
}
