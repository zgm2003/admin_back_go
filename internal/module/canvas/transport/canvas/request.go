package canvas

type listPromptsRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	Category    string `form:"category"`
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
	AgentID           int64    `json:"agent_id"`
	Prompt            string   `json:"prompt"`
	Size              string   `json:"size"`
	Quality           string   `json:"quality"`
	OutputFormat      string   `json:"output_format"`
	OutputCompression *int     `json:"output_compression"`
	Moderation        string   `json:"moderation"`
	N                 int      `json:"n"`
	InputAssetIDs     []uint64 `json:"input_asset_ids"`
	MaskAssetID       uint64   `json:"mask_asset_id"`
	MaskTargetAssetID uint64   `json:"mask_target_asset_id"`
}

type videoGenerationRequest struct {
	AgentID         int64  `json:"agent_id"`
	ModelID         string `json:"model"`
	Prompt          string `json:"prompt"`
	DurationSeconds int    `json:"duration_seconds"`
	Size            string `json:"size"`
	ResolutionName  string `json:"resolution_name"`
}
