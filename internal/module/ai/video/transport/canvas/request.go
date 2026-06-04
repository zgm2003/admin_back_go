package canvas

type videoGenerationRequest struct {
	AgentID         int64  `json:"agent_id" binding:"required,gt=0"`
	ModelID         string `json:"model" binding:"omitempty,max=128"`
	Prompt          string `json:"prompt" binding:"required,max=20000"`
	DurationSeconds int    `json:"duration_seconds" binding:"omitempty,gte=0,lte=60"`
	Size            string `json:"size" binding:"omitempty,max=64"`
	ResolutionName  string `json:"resolution_name" binding:"omitempty,max=64"`
}
