package canvas

type listAssetsRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	Type        string `form:"type"`
}

type assetRequest struct {
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	TagsJSON    string `json:"tags_json"`
	Status      int    `json:"status"`
}
