package canvas

type listPromptsRequest struct {
	CurrentPage int      `form:"current_page"`
	PageSize    int      `form:"page_size"`
	Keyword     string   `form:"keyword"`
	Category    string   `form:"category"`
	Tag         []string `form:"tag"`
}
