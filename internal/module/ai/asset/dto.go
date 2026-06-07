package asset

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
	Type        string
	Status      int
	IsDel       int
}

type Input struct {
	Slug        string
	Type        string
	Category    string
	Title       string
	CoverURL    string
	Description string
	Content     string
	URL         string
	TagsJSON    string
	Status      int
}

type ListResponse struct {
	List []Item `json:"list"`
	Page Page   `json:"page"`
}

type Item struct {
	ID          int64  `json:"id"`
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
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
