package airun

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type JSONObject map[string]any

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	RunStatusArr []dict.Option[int] `json:"run_status_arr"`
	AgentArr     []dict.Option[int] `json:"agentArr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	RunStatus   *int
	UserID      *int64
	RequestID   string
	AgentID     *int64
	DateStart   string
	DateEnd     string
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID                int64   `json:"id"`
	RequestID         string  `json:"request_id"`
	UserID            int64   `json:"user_id"`
	AgentID           int64   `json:"agent_id"`
	AgentName         string  `json:"agent_name"`
	ConversationID    int64   `json:"conversation_id"`
	ConversationTitle string  `json:"conversation_title"`
	RunStatus         int     `json:"run_status"`
	RunStatusName     string  `json:"run_status_name"`
	ModelSnapshot     string  `json:"model_snapshot"`
	PromptTokens      *int    `json:"prompt_tokens"`
	CompletionTokens  *int    `json:"completion_tokens"`
	TotalTokens       *int    `json:"total_tokens"`
	LatencyMS         *int    `json:"latency_ms"`
	LatencyStr        string  `json:"latency_str"`
	ErrorMsg          *string `json:"error_msg"`
	CreatedAt         string  `json:"created_at"`
}

type MessageSummary struct {
	ID        int64      `json:"id"`
	Content   string     `json:"content"`
	MetaJSON  JSONObject `json:"meta_json"`
	CreatedAt string     `json:"created_at"`
}

type StepItem struct {
	ID            int64      `json:"id"`
	StepNo        int        `json:"step_no"`
	StepType      int        `json:"step_type"`
	StepTypeName  string     `json:"step_type_name"`
	AgentID       int64      `json:"agent_id"`
	AgentName     string     `json:"agent_name"`
	ModelSnapshot *string    `json:"model_snapshot"`
	Status        int        `json:"status"`
	StatusName    string     `json:"status_name"`
	ErrorMsg      *string    `json:"error_msg"`
	LatencyMS     *int       `json:"latency_ms"`
	LatencyStr    string     `json:"latency_str"`
	PayloadJSON   JSONObject `json:"payload_json"`
	CreatedAt     string     `json:"created_at"`
}

type DetailResponse struct {
	ID                int64           `json:"id"`
	RequestID         string          `json:"request_id"`
	UserID            int64           `json:"user_id"`
	Username          string          `json:"username"`
	AgentID           int64           `json:"agent_id"`
	AgentName         string          `json:"agent_name"`
	ConversationID    int64           `json:"conversation_id"`
	ConversationTitle string          `json:"conversation_title"`
	RunStatus         int             `json:"run_status"`
	RunStatusName     string          `json:"run_status_name"`
	ModelSnapshot     string          `json:"model_snapshot"`
	PromptTokens      *int            `json:"prompt_tokens"`
	CompletionTokens  *int            `json:"completion_tokens"`
	TotalTokens       *int            `json:"total_tokens"`
	LatencyMS         *int            `json:"latency_ms"`
	LatencyStr        string          `json:"latency_str"`
	ErrorMsg          *string         `json:"error_msg"`
	MetaJSON          JSONObject      `json:"meta_json"`
	UserMessage       *MessageSummary `json:"user_message"`
	AssistantMessage  *MessageSummary `json:"assistant_message"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	Steps             []StepItem      `json:"steps"`
}

type StatsFilter struct {
	DateStart string
	DateEnd   string
	AgentID   *int64
	UserID    *int64
}

type StatsResponse struct {
	DateRange DateRange    `json:"date_range"`
	Summary   StatsSummary `json:"summary"`
}

type DateRange struct {
	Start *string `json:"start"`
	End   *string `json:"end"`
}

type StatsSummary struct {
	TotalRuns             int64   `json:"total_runs"`
	SuccessRate           float64 `json:"success_rate"`
	FailRuns              int64   `json:"fail_runs"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	AvgLatencyMS          int64   `json:"avg_latency_ms"`
}

type StatsMetricItem struct {
	TotalRuns             int64 `json:"total_runs"`
	TotalTokens           int64 `json:"total_tokens"`
	TotalPromptTokens     int64 `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
	AvgLatencyMS          int64 `json:"avg_latency_ms"`
}

type StatsByDateItem struct {
	Date string `json:"date"`
	StatsMetricItem
}

type StatsByAgentItem struct {
	AgentName string `json:"agent_name"`
	StatsMetricItem
}

type StatsByUserItem struct {
	Username string `json:"username"`
	StatsMetricItem
}

type StatsByDateResponse struct {
	List []StatsByDateItem `json:"list"`
	Page Page              `json:"page"`
}
type StatsByAgentResponse struct {
	List []StatsByAgentItem `json:"list"`
	Page Page               `json:"page"`
}
type StatsByUserResponse struct {
	List []StatsByUserItem `json:"list"`
	Page Page              `json:"page"`
}

type AgentOptionRow struct {
	ID   int64
	Name string
}

type ListRow struct {
	ID                int64
	RequestID         string
	UserID            int64
	AgentID           int64
	AgentName         string
	ConversationID    int64
	ConversationTitle string
	RunStatus         int
	ModelSnapshot     string
	PromptTokens      *int
	CompletionTokens  *int
	TotalTokens       *int
	LatencyMS         *int
	ErrorMsg          *string
	CreatedAt         time.Time
}

type RunDetailRow struct {
	ID                int64
	RequestID         string
	UserID            int64
	Username          string
	AgentID           int64
	AgentName         string
	ConversationID    int64
	ConversationTitle string
	RunStatus         int
	ModelSnapshot     string
	PromptTokens      *int
	CompletionTokens  *int
	TotalTokens       *int
	LatencyMS         *int
	ErrorMsg          *string
	MetaJSON          *string
	UserMessage       *MessageSummary
	AssistantMessage  *MessageSummary
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type StepRow struct {
	ID            int64
	StepNo        int
	StepType      int
	AgentID       int64
	AgentName     string
	ModelSnapshot *string
	Status        int
	ErrorMsg      *string
	LatencyMS     *int
	PayloadJSON   *string
	CreatedAt     time.Time
}

type StatsSummaryRow struct {
	TotalRuns        int64
	SuccessRuns      int64
	FailRuns         int64
	TotalTokens      int64
	PromptTokens     int64
	CompletionTokens int64
	AvgLatencyMS     int64
}

type StatsMetricRow struct {
	TotalRuns        int64
	TotalTokens      int64
	PromptTokens     int64
	CompletionTokens int64
	AvgLatencyMS     int64
}

type StatsListQuery struct {
	CurrentPage int
	PageSize    int
	DateStart   string
	DateEnd     string
	AgentID     *int64
	UserID      *int64
}

type StatsByDateRow struct {
	Date string
	StatsMetricRow
}
type StatsByAgentRow struct {
	AgentName string
	StatsMetricRow
}
type StatsByUserRow struct {
	Username string
	StatsMetricRow
}

type Repository interface {
	AgentOptions(ctx context.Context) ([]AgentOptionRow, error)
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Detail(ctx context.Context, id int64) (*RunDetailRow, error)
	Steps(ctx context.Context, runID int64) ([]StepRow, error)
	StatsSummary(ctx context.Context, query StatsFilter) (StatsSummaryRow, error)
	StatsByDate(ctx context.Context, query StatsListQuery) ([]StatsByDateRow, int64, error)
	StatsByAgent(ctx context.Context, query StatsListQuery) ([]StatsByAgentRow, int64, error)
	StatsByUser(ctx context.Context, query StatsListQuery) ([]StatsByUserRow, int64, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error)
	Stats(ctx context.Context, query StatsFilter) (*StatsResponse, *apperror.Error)
	StatsByDate(ctx context.Context, query StatsListQuery) (*StatsByDateResponse, *apperror.Error)
	StatsByAgent(ctx context.Context, query StatsListQuery) (*StatsByAgentResponse, *apperror.Error)
	StatsByUser(ctx context.Context, query StatsListQuery) (*StatsByUserResponse, *apperror.Error)
}
