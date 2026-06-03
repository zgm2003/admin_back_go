package canvas

type imageGenerationRequest struct {
	AgentID           uint64   `json:"agent_id" form:"agent_id" binding:"required,gt=0"`
	Prompt            string   `json:"prompt" form:"prompt" binding:"required,max=20000"`
	Size              string   `json:"size" form:"size" binding:"omitempty,max=32"`
	Quality           string   `json:"quality" form:"quality" binding:"omitempty,max=16"`
	OutputFormat      string   `json:"output_format" form:"output_format" binding:"omitempty,max=16"`
	OutputCompression *int     `json:"output_compression" form:"output_compression" binding:"omitempty,gte=0,lte=100"`
	Moderation        string   `json:"moderation" form:"moderation" binding:"omitempty,max=16"`
	N                 int      `json:"n" form:"n" binding:"omitempty,min=1,max=15"`
	InputAssetIDs     []uint64 `json:"input_asset_ids" form:"input_asset_ids" binding:"omitempty,dive,gt=0"`
	MaskAssetID       uint64   `json:"mask_asset_id" form:"mask_asset_id" binding:"omitempty,gt=0"`
	MaskTargetAssetID uint64   `json:"mask_target_asset_id" form:"mask_target_asset_id" binding:"omitempty,gt=0"`
}

type imageGenerationResponse struct {
	TaskID uint64 `json:"task_id"`
	Status string `json:"status"`
}
