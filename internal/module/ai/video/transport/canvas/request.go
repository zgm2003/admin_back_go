package canvas

type videoGenerationRequest struct {
	AgentID         int64  `json:"agent_id" form:"agent_id" binding:"required,gt=0"`
	Prompt          string `json:"prompt" form:"prompt" binding:"required,max=20000"`
	DurationSeconds int    `json:"duration_seconds" form:"duration_seconds" binding:"omitempty,gte=0,lte=60"`
	Size            string `json:"size" form:"size" binding:"omitempty,max=64"`
	ResolutionName  string `json:"resolution_name" form:"resolution_name" binding:"omitempty,max=64"`
	GenerateAudio   *bool  `json:"generate_audio" form:"generate_audio" binding:"omitempty"`
	Watermark       *bool  `json:"watermark" form:"watermark" binding:"omitempty"`
}
