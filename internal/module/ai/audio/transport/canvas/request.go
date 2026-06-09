package canvas

type audioGenerationRequest struct {
	AgentID        int64    `json:"agent_id" binding:"required"`
	Prompt         string   `json:"prompt" binding:"required"`
	Voice          string   `json:"voice" binding:"omitempty,max=64"`
	ResponseFormat string   `json:"response_format" binding:"omitempty,max=32"`
	Speed          *float64 `json:"speed" binding:"omitempty"`
	Instructions   string   `json:"instructions" binding:"omitempty,max=4000"`
}
