package queuemonitor

type QueueItem struct {
	Name           string `json:"name"`
	Label          string `json:"label"`
	Group          string `json:"group"`
	Waiting        int    `json:"waiting"`
	Delayed        int    `json:"delayed"`
	Failed         int    `json:"failed"`
	Pending        int    `json:"pending"`
	Active         int    `json:"active"`
	Scheduled      int    `json:"scheduled"`
	Retry          int    `json:"retry"`
	Archived       int    `json:"archived"`
	Completed      int    `json:"completed"`
	Processed      int    `json:"processed"`
	FailedToday    int    `json:"failed_today"`
	ProcessedTotal int    `json:"processed_total"`
	FailedTotal    int    `json:"failed_total"`
	Paused         bool   `json:"paused"`
	LatencyMs      int64  `json:"latency_ms"`
}

type FailedListQuery struct {
	Queue       string
	CurrentPage int
	PageSize    int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type FailedListResponse struct {
	List []FailedTaskItem `json:"list"`
	Page Page             `json:"page"`
}

type FailedTaskItem struct {
	Index        int            `json:"index"`
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	State        string         `json:"state"`
	Data         map[string]any `json:"data"`
	MaxAttempts  int            `json:"max_attempts"`
	Attempts     int            `json:"attempts"`
	Error        string         `json:"error"`
	Raw          string         `json:"raw"`
	LastFailedAt string         `json:"last_failed_at"`
}
