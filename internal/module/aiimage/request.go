package aiimage

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Status      string `form:"status" binding:"omitempty,max=16"`
	IsFavorite  int    `form:"is_favorite" binding:"omitempty,oneof=1 2"`
}

type registerAssetRequest struct {
	StorageProvider string `json:"storage_provider" binding:"required,max=32"`
	StorageKey      string `json:"storage_key" binding:"max=512"`
	StorageURL      string `json:"storage_url" binding:"required,max=1000"`
	MimeType        string `json:"mime_type" binding:"required,max=64"`
	Width           int    `json:"width" binding:"omitempty,gte=0"`
	Height          int    `json:"height" binding:"omitempty,gte=0"`
	SizeBytes       int64  `json:"size_bytes" binding:"omitempty,gte=0"`
	SourceType      string `json:"source_type" binding:"required,max=16"`
}

type createTaskRequest struct {
	AgentID           uint64   `json:"agent_id" binding:"required,gt=0"`
	Prompt            string   `json:"prompt" binding:"required,max=20000"`
	Size              string   `json:"size" binding:"omitempty,max=32"`
	Quality           string   `json:"quality" binding:"omitempty,max=16"`
	OutputFormat      string   `json:"output_format" binding:"omitempty,max=16"`
	OutputCompression *int     `json:"output_compression" binding:"omitempty,gte=0,lte=100"`
	Moderation        string   `json:"moderation" binding:"omitempty,max=16"`
	N                 int      `json:"n" binding:"omitempty,min=1,max=4"`
	InputAssetIDs     []uint64 `json:"input_asset_ids" binding:"omitempty,dive,gt=0"`
	MaskAssetID       uint64   `json:"mask_asset_id" binding:"omitempty,gt=0"`
	MaskTargetAssetID uint64   `json:"mask_target_asset_id" binding:"omitempty,gt=0"`
}

type favoriteRequest struct {
	IsFavorite int `json:"is_favorite" binding:"required,oneof=1 2"`
}
