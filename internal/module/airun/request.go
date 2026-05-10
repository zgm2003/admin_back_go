package airun

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Status      string `form:"status" binding:"omitempty,oneof=running success failed canceled timeout"`
	UserID      *int64 `form:"user_id" binding:"omitempty,min=1"`
	RequestID   string `form:"request_id" binding:"max=64"`
	AgentID     *int64 `form:"agent_id" binding:"omitempty,min=1"`
	ProviderID  *int64 `form:"provider_id" binding:"omitempty,min=1"`
	DateStart   string `form:"date_start" binding:"omitempty,max=20"`
	DateEnd     string `form:"date_end" binding:"omitempty,max=20"`
}

type statsRequest struct {
	DateStart  string `form:"date_start" binding:"omitempty,max=20"`
	DateEnd    string `form:"date_end" binding:"omitempty,max=20"`
	AgentID    *int64 `form:"agent_id" binding:"omitempty,min=1"`
	ProviderID *int64 `form:"provider_id" binding:"omitempty,min=1"`
	UserID     *int64 `form:"user_id" binding:"omitempty,min=1"`
}

type statsListRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	DateStart   string `form:"date_start" binding:"omitempty,max=20"`
	DateEnd     string `form:"date_end" binding:"omitempty,max=20"`
	AgentID     *int64 `form:"agent_id" binding:"omitempty,min=1"`
	ProviderID  *int64 `form:"provider_id" binding:"omitempty,min=1"`
	UserID      *int64 `form:"user_id" binding:"omitempty,min=1"`
}
