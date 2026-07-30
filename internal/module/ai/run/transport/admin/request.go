package admin

type pageInitRequest struct {
	DateStart string `form:"date_start" binding:"omitempty,len=10,datetime=2006-01-02"`
	DateEnd   string `form:"date_end" binding:"omitempty,len=10,datetime=2006-01-02"`
}

type listRequest struct {
	CurrentPage    int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Platform       string `form:"platform" binding:"omitempty,max=32"`
	Status         string `form:"status" binding:"omitempty,oneof=running success failed canceled timeout outcome_unknown"`
	UserID         *int64 `form:"user_id" binding:"omitempty,min=1"`
	RequestID      string `form:"request_id" binding:"max=128"`
	AgentID        *int64 `form:"agent_id" binding:"omitempty,min=1"`
	ProviderID     *int64 `form:"provider_id" binding:"omitempty,min=1"`
	ModelID        string `form:"model_id" binding:"omitempty,max=191"`
	BillingStatus  string `form:"billing_status" binding:"omitempty,max=16"`
	BillingReason  string `form:"billing_reason" binding:"omitempty,max=32"`
	ErrorCode      string `form:"error_code" binding:"omitempty,max=128"`
	ToolCode       string `form:"tool_code" binding:"omitempty,max=128"`
	RunAnomaly     string `form:"run_anomaly" binding:"omitempty,max=32"`
	BillingAnomaly string `form:"billing_anomaly" binding:"omitempty,max=32"`
	UserFeedback   string `form:"user_feedback" binding:"omitempty,oneof=liked unliked"`
	AnomalyAsOf    string `form:"anomaly_as_of" binding:"omitempty,max=64"`
	DateStart      string `form:"date_start" binding:"omitempty,max=20"`
	DateEnd        string `form:"date_end" binding:"omitempty,max=20"`
}

type dashboardRequest struct {
	DateStart  string `form:"date_start" binding:"omitempty,len=10,datetime=2006-01-02"`
	DateEnd    string `form:"date_end" binding:"omitempty,len=10,datetime=2006-01-02"`
	Platform   string `form:"platform" binding:"omitempty,max=32"`
	ModelID    string `form:"model_id" binding:"omitempty,max=191"`
	AgentID    *int64 `form:"agent_id" binding:"omitempty,min=1"`
	ProviderID *int64 `form:"provider_id" binding:"omitempty,min=1"`
	UserID     *int64 `form:"user_id" binding:"omitempty,min=1"`
}
