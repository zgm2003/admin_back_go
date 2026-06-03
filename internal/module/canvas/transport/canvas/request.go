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

type videoGenerationRequest struct {
	AgentID         int64  `json:"agent_id"`
	ModelID         string `json:"model"`
	Prompt          string `json:"prompt"`
	DurationSeconds int    `json:"duration_seconds"`
	Size            string `json:"size"`
	ResolutionName  string `json:"resolution_name"`
}
