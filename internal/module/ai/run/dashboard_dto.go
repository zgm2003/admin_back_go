package airun

import "time"

type DashboardFilter struct {
	RequestID  string
	DateStart  string
	DateEnd    string
	Platform   string
	ModelID    string
	AgentID    *int64
	ProviderID *int64
	UserID     *int64
}

type DashboardQuery struct {
	StartAt      time.Time
	EndExclusive time.Time
	GeneratedAt  time.Time
	StaleBefore  time.Time
	Platform     string
	ModelID      string
	AgentID      *int64
	ProviderID   *int64
	UserID       *int64
}

type DashboardResponse struct {
	GeneratedAt string               `json:"generated_at"`
	Timezone    string               `json:"timezone"`
	DateRange   DashboardDateRange   `json:"date_range"`
	Summary     DashboardSummary     `json:"summary"`
	Performance DashboardPerformance `json:"performance"`
	Billing     DashboardBilling     `json:"billing"`
	Anomalies   DashboardAnomalies   `json:"anomalies"`
	Trend       []DashboardTrendItem `json:"trend"`
	Breakdowns  DashboardBreakdowns  `json:"breakdowns"`
}

type DashboardPercentile struct {
	SampleCount        int64 `json:"sample_count"`
	InsufficientSample bool  `json:"insufficient_sample"`
	P50MS              int64 `json:"p50_ms"`
	P95MS              int64 `json:"p95_ms"`
}

type DashboardDateRange struct {
	StartAt      string `json:"start_at"`
	EndExclusive string `json:"end_exclusive"`
}

type DashboardSummary struct {
	TotalRuns          int64   `json:"total_runs"`
	TerminalRuns       int64   `json:"terminal_runs"`
	InProgressRuns     int64   `json:"in_progress_runs"`
	SuccessRuns        int64   `json:"success_runs"`
	FailedRuns         int64   `json:"failed_runs"`
	TimeoutRuns        int64   `json:"timeout_runs"`
	OutcomeUnknownRuns int64   `json:"outcome_unknown_runs"`
	CanceledRuns       int64   `json:"canceled_runs"`
	SuccessDenominator int64   `json:"success_denominator"`
	SuccessRate        float64 `json:"success_rate"`
	PromptTokens       int64   `json:"prompt_tokens"`
	CompletionTokens   int64   `json:"completion_tokens"`
	TotalTokens        int64   `json:"total_tokens"`
}

type DashboardPerformance struct {
	TTFT     DashboardPercentile `json:"ttft"`
	EndToEnd DashboardPercentile `json:"end_to_end"`
}

type DashboardBilling struct {
	SettledRuns    int64  `json:"settled_runs"`
	ActualAmount   string `json:"actual_amount"`
	ReleasedRuns   int64  `json:"released_runs"`
	ReleasedAmount string `json:"released_amount"`
	UnbilledRuns   int64  `json:"unbilled_runs"`
}

type DashboardAnomalyItem struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

type DashboardAnomalies struct {
	RunTotal     int64                  `json:"run_total"`
	BillingTotal int64                  `json:"billing_total"`
	RunItems     []DashboardAnomalyItem `json:"run_items"`
	BillingItems []DashboardAnomalyItem `json:"billing_items"`
}

type DashboardRepositoryResult struct {
	Summary          DashboardSummaryRow
	Performance      DashboardPerformanceRow
	Billing          DashboardBillingRow
	RunAnomalies     []DashboardCountRow
	BillingAnomalies []DashboardCountRow
	Trend            []DashboardTrendRow
	Attributions     []DashboardAttributionRow
	Errors           []DashboardErrorRow
	Tools            []DashboardToolRow
}

type DashboardSummaryRow struct {
	TotalRuns          int64
	RunningRuns        int64
	SuccessRuns        int64
	FailedRuns         int64
	CanceledRuns       int64
	TimeoutRuns        int64
	OutcomeUnknownRuns int64
	PromptTokens       int64
	CompletionTokens   int64
	TotalTokens        int64
}

type DashboardDistributionRow struct {
	SampleCount int64
	P50MS       int64
	P95MS       int64
}

type DashboardPerformanceRow struct {
	TTFT     DashboardDistributionRow
	EndToEnd DashboardDistributionRow
}

type DashboardBillingRow struct {
	SettledRuns   int64
	ActualUnits   int64
	ReleasedRuns  int64
	ReleasedUnits int64
	UnbilledRuns  int64
}

type DashboardCountRow struct {
	Code  string
	Count int64
}

type DashboardTrendRow struct {
	Date               string
	TotalRuns          int64
	RunningRuns        int64
	SuccessRuns        int64
	FailedRuns         int64
	CanceledRuns       int64
	TimeoutRuns        int64
	OutcomeUnknownRuns int64
	ActualUnits        int64
	TTFT               DashboardDistributionRow
	EndToEnd           DashboardDistributionRow
}

type DashboardAttributionRow struct {
	Dimension           string
	Key                 string
	ID                  int64
	Name                string
	TotalRuns           int64
	SuccessRuns         int64
	FailedRuns          int64
	TimeoutRuns         int64
	OutcomeUnknownRuns  int64
	TotalTokens         int64
	ActualUnits         int64
	RunAnomalyCount     int64
	BillingAnomalyCount int64
}

type DashboardErrorRow struct {
	ErrorCode string
	Count     int64
}

type DashboardToolRow struct {
	ToolCode     string
	ToolName     string
	TotalCalls   int64
	SuccessCalls int64
	FailedCalls  int64
	TimeoutCalls int64
	Duration     DashboardDistributionRow
}

type DashboardAttributionMetrics struct {
	TotalRuns           int64   `json:"total_runs"`
	SuccessRuns         int64   `json:"success_runs"`
	SuccessDenominator  int64   `json:"success_denominator"`
	SuccessRate         float64 `json:"success_rate"`
	TotalTokens         int64   `json:"total_tokens"`
	ActualAmount        string  `json:"actual_amount"`
	RunAnomalyCount     int64   `json:"run_anomaly_count"`
	BillingAnomalyCount int64   `json:"billing_anomaly_count"`
}

type DashboardModelBreakdown struct {
	ModelID          string `json:"model_id"`
	ModelDisplayName string `json:"model_display_name"`
	Historical       bool   `json:"historical"`
	DashboardAttributionMetrics
}

type DashboardProviderBreakdown struct {
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	DashboardAttributionMetrics
}

type DashboardAgentBreakdown struct {
	AgentID   int64  `json:"agent_id"`
	AgentName string `json:"agent_name"`
	DashboardAttributionMetrics
}

type DashboardUserBreakdown struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	DashboardAttributionMetrics
}

type DashboardErrorBreakdown struct {
	ErrorCode string `json:"error_code"`
	Count     int64  `json:"count"`
}

type DashboardToolBreakdown struct {
	ToolCode           string              `json:"tool_code"`
	ToolName           string              `json:"tool_name"`
	TotalCalls         int64               `json:"total_calls"`
	SuccessCalls       int64               `json:"success_calls"`
	FailedCalls        int64               `json:"failed_calls"`
	TimeoutCalls       int64               `json:"timeout_calls"`
	SuccessDenominator int64               `json:"success_denominator"`
	SuccessRate        float64             `json:"success_rate"`
	Duration           DashboardPercentile `json:"duration"`
}

type DashboardTrendItem struct {
	Date               string              `json:"date"`
	TotalRuns          int64               `json:"total_runs"`
	InProgressRuns     int64               `json:"in_progress_runs"`
	SuccessRuns        int64               `json:"success_runs"`
	FailedRuns         int64               `json:"failed_runs"`
	CanceledRuns       int64               `json:"canceled_runs"`
	TimeoutRuns        int64               `json:"timeout_runs"`
	OutcomeUnknownRuns int64               `json:"outcome_unknown_runs"`
	SuccessDenominator int64               `json:"success_denominator"`
	SuccessRate        float64             `json:"success_rate"`
	ActualAmount       string              `json:"actual_amount"`
	TTFT               DashboardPercentile `json:"ttft"`
	EndToEnd           DashboardPercentile `json:"end_to_end"`
}

type DashboardBreakdowns struct {
	Models    []DashboardModelBreakdown    `json:"models"`
	Providers []DashboardProviderBreakdown `json:"providers"`
	Agents    []DashboardAgentBreakdown    `json:"agents"`
	Users     []DashboardUserBreakdown     `json:"users"`
	Errors    []DashboardErrorBreakdown    `json:"errors"`
	Tools     []DashboardToolBreakdown     `json:"tools"`
}
