package prompt

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	Category    string
	Tags        []string
	Status      int
	IsDel       int
}

type Input struct {
	Slug      string
	Category  string
	Title     string
	CoverURL  string
	Prompt    string
	Preview   string
	TagsJSON  string
	SourceURL string
	Status    int
}

type ListResponse struct {
	List []Item `json:"list"`
	Page Page   `json:"page"`
}

type Item struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Category  string `json:"category"`
	Title     string `json:"title"`
	CoverURL  string `json:"cover_url"`
	Prompt    string `json:"prompt"`
	Preview   string `json:"preview"`
	TagsJSON  string `json:"tags_json"`
	SourceURL string `json:"source_url"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
