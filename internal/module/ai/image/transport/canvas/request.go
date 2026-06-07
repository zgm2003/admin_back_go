package canvas

type imageGenerationRequest struct {
	AgentID           uint64 `json:"agent_id" form:"agent_id" binding:"required,gt=0"`
	Prompt            string `json:"prompt" form:"prompt" binding:"required,max=20000"`
	Size              string `json:"size" form:"size" binding:"omitempty,max=32"`
	Quality           string `json:"quality" form:"quality" binding:"omitempty,max=16"`
	OutputFormat      string `json:"output_format" form:"output_format" binding:"omitempty,max=16"`
	OutputCompression *int   `json:"output_compression" form:"output_compression" binding:"omitempty,gte=0,lte=100"`
	Moderation        string `json:"moderation" form:"moderation" binding:"omitempty,max=16"`
	N                 int    `json:"n" form:"n" binding:"omitempty,min=1,max=15"`
}

type imageGenerationResponse struct {
	TaskID uint64 `json:"task_id"`
	Status string `json:"status"`
}

type listRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1"`
	Status   string `form:"status" binding:"omitempty,oneof=pending running success failed"`
}
