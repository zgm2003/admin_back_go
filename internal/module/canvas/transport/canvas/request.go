package canvas

type listPromptsRequest struct {
	CurrentPage int      `form:"current_page"`
	PageSize    int      `form:"page_size"`
	Keyword     string   `form:"keyword"`
	Category    string   `form:"category"`
	Tag         []string `form:"tag"`
}

type listAssetsRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	Type        string `form:"type"`
}

type chatCompletionRequest struct {
	AgentID int64  `json:"agent_id"`
	ModelID string `json:"model"`
	Message string `json:"message"`
}

type imageGenerationRequest struct {
	AgentID           int64    `json:"agent_id" form:"agent_id"`
	Prompt            string   `json:"prompt" form:"prompt"`
	Size              string   `json:"size" form:"size"`
	Quality           string   `json:"quality" form:"quality"`
	OutputFormat      string   `json:"output_format" form:"output_format"`
	OutputCompression *int     `json:"output_compression" form:"output_compression"`
	Moderation        string   `json:"moderation" form:"moderation"`
	N                 int      `json:"n" form:"n"`
	InputAssetIDs     []uint64 `json:"input_asset_ids" form:"input_asset_ids"`
	MaskAssetID       uint64   `json:"mask_asset_id" form:"mask_asset_id"`
	MaskTargetAssetID uint64   `json:"mask_target_asset_id" form:"mask_target_asset_id"`
}

type videoGenerationRequest struct {
	AgentID         int64  `json:"agent_id"`
	ModelID         string `json:"model"`
	Prompt          string `json:"prompt"`
	DurationSeconds int    `json:"duration_seconds"`
	Size            string `json:"size"`
	ResolutionName  string `json:"resolution_name"`
}
