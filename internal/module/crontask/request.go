package crontask

type listRequest struct {
	CurrentPage    int    `form:"current_page" binding:"required,min=1"`
	PageSize       int    `form:"page_size" binding:"required,min=1,max=50"`
	Title          string `form:"title" binding:"max=100"`
	Name           string `form:"name" binding:"max=100"`
	Status         *int   `form:"status" binding:"omitempty,common_status"`
	RegistryStatus string `form:"registry_status" binding:"omitempty,oneof=registered missing disabled invalid_cron"`
}

type saveRequest struct {
	Name         string `json:"name" binding:"required,max=100"`
	Title        string `json:"title" binding:"required,max=100"`
	Description  string `json:"description" binding:"max=500"`
	Cron         string `json:"cron" binding:"required,max=100"`
	CronReadable string `json:"cron_readable" binding:"max=100"`
	Handler      string `json:"handler" binding:"max=255"`
	Status       int    `json:"status" binding:"required,common_status"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}

type batchDeleteRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type logsRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Status      *int   `form:"status" binding:"omitempty,oneof=1 2 3"`
	StartDate   string `form:"start_date" binding:"max=20"`
	EndDate     string `form:"end_date" binding:"max=20"`
}
