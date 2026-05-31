package canvas

type transactionListRequest struct {
	CurrentPage int    `form:"current_page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	Direction   string `form:"direction"`
	SourceType  string `form:"source_type"`
	DateStart   string `form:"date_start"`
	DateEnd     string `form:"date_end"`
}
