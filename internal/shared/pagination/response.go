package pagination

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type Result[T any] struct {
	List []T  `json:"list"`
	Page Page `json:"page"`
}
