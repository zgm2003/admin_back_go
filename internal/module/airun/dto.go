package airun

import (
	"context"
	"encoding/json"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type JSONObject = json.RawMessage

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	StatusArr   []dict.Option[string] `json:"status_arr"`
	ProviderArr []dict.Option[int]    `json:"providerArr"`
	AgentArr    []dict.Option[int]    `json:"agentArr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Status      string
	UserID      *int64
	RequestID   string
	AgentID     *int64
	ProviderID  *int64
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
	ID                int64  `json:"id"`
	RequestID         string `json:"request_id"`
	UserID            int64  `json:"user_id"`
	AgentID           int64  `json:"agent_id"`
	AgentName         string `json:"agent_name"`
	ProviderID        int64  `json:"provider_id"`
	ProviderName      string `json:"provider_name"`
	ConversationID    int64  `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	Status            string `json:"status"`
	StatusName        string `json:"status_name"`
	ModelID           string `json:"model_id"`
	ModelDisplayName  string `json:"model_display_name"`
	PromptTokens      uint   `json:"prompt_tokens"`
	CompletionTokens  uint   `json:"completion_tokens"`
	TotalTokens       uint   `json:"total_tokens"`
	DurationMS        *uint  `json:"duration_ms"`
	DurationText      string `json:"duration_text"`
	ErrorMessage      string `json:"error_message"`
	CreatedAt         string `json:"created_at"`
}

type MessageSummary struct {
	ID          int64      `json:"id"`
	Role        int        `json:"role"`
	ContentType string     `json:"content_type"`
	Content     string     `json:"content"`
	MetaJSON    JSONObject `json:"meta_json"`
	CreatedAt   string     `json:"created_at"`
}

type EventItem struct {
	ID            int64  `json:"id"`
	Seq           uint   `json:"seq"`
	EventType     string `json:"event_type"`
	EventTypeName string `json:"event_type_name"`
	Message       string `json:"message"`
	ElapsedMS     *uint  `json:"elapsed_ms"`
	ElapsedText   string `json:"elapsed_text"`
	CreatedAt     string `json:"created_at"`
}

type ToolCallItem struct {
	ID            int64      `json:"id"`
	ToolID        int64      `json:"tool_id"`
	ToolCode      string     `json:"tool_code"`
	ToolName      string     `json:"tool_name"`
	CallID        *string    `json:"call_id"`
	Status        string     `json:"status"`
	ArgumentsJSON JSONObject `json:"arguments_json"`
	ResultJSON    JSONObject `json:"result_json"`
	ErrorMessage  string     `json:"error_message"`
	DurationMS    *uint      `json:"duration_ms"`
	StartedAt     string     `json:"started_at"`
	FinishedAt    string     `json:"finished_at"`
}

type DetailResponse struct {
	ID                int64           `json:"id"`
	RequestID         string          `json:"request_id"`
	UserID            int64           `json:"user_id"`
	Username          string          `json:"username"`
	AgentID           int64           `json:"agent_id"`
	AgentName         string          `json:"agent_name"`
	ProviderID        int64           `json:"provider_id"`
	ProviderName      string          `json:"provider_name"`
	ConversationID    int64           `json:"conversation_id"`
	ConversationTitle string          `json:"conversation_title"`
	Status            string          `json:"status"`
	StatusName        string          `json:"status_name"`
	ModelID           string          `json:"model_id"`
	ModelDisplayName  string          `json:"model_display_name"`
	PromptTokens      uint            `json:"prompt_tokens"`
	CompletionTokens  uint            `json:"completion_tokens"`
	TotalTokens       uint            `json:"total_tokens"`
	DurationMS        *uint           `json:"duration_ms"`
	DurationText      string          `json:"duration_text"`
	ErrorMessage      string          `json:"error_message"`
	UserMessage       *MessageSummary `json:"user_message"`
	AssistantMessage  *MessageSummary `json:"assistant_message"`
	Events            []EventItem     `json:"events"`
	ToolCalls         []ToolCallItem  `json:"tool_calls"`
	StartedAt         string          `json:"started_at"`
	FinishedAt        string          `json:"finished_at"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

type StatsFilter struct {
	DateStart  string
	DateEnd    string
	AgentID    *int64
	ProviderID *int64
	UserID     *int64
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
	AvgDurationMS         int64   `json:"avg_duration_ms"`
}

type StatsMetricItem struct {
	TotalRuns             int64 `json:"total_runs"`
	TotalTokens           int64 `json:"total_tokens"`
	TotalPromptTokens     int64 `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
	AvgDurationMS         int64 `json:"avg_duration_ms"`
}

type StatsByDateItem struct {
	Date string `json:"date"`
	StatsMetricItem
}

type StatsByAgentItem struct {
	AgentID   int64  `json:"agent_id"`
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

type OptionRow struct {
	ID   int64
	Name string
}

type ListRow struct {
	ID                int64
	RequestID         string
	UserID            int64
	AgentID           int64
	AgentName         string
	ProviderID        int64
	ProviderName      string
	ConversationID    int64
	ConversationTitle string
	Status            string
	ModelID           string
	ModelDisplayName  string
	PromptTokens      uint
	CompletionTokens  uint
	TotalTokens       uint
	DurationMS        *uint
	ErrorMessage      string
	CreatedAt         time.Time
}

type RunDetailRow struct {
	ID                int64
	RequestID         string
	UserID            int64
	Username          string
	AgentID           int64
	AgentName         string
	ProviderID        int64
	ProviderName      string
	ConversationID    int64
	ConversationTitle string
	Status            string
	ModelID           string
	ModelDisplayName  string
	PromptTokens      uint
	CompletionTokens  uint
	TotalTokens       uint
	DurationMS        *uint
	ErrorMessage      string
	UserMessage       *MessageSummary `gorm:"-"`
	AssistantMessage  *MessageSummary `gorm:"-"`
	StartedAt         *time.Time
	FinishedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type EventRow struct {
	ID        int64
	Seq       uint
	EventType string
	Message   string
	CreatedAt time.Time
}

type ToolCallRow struct {
	ID            int64
	ToolID        int64
	ToolCode      string
	ToolName      string
	CallID        *string
	Status        string
	ArgumentsJSON string
	ResultJSON    *string
	ErrorMessage  string
	DurationMS    *uint
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type StatsSummaryRow struct {
	TotalRuns        int64
	SuccessRuns      int64
	FailRuns         int64
	TotalTokens      int64
	PromptTokens     int64
	CompletionTokens int64
	AvgDurationMS    int64
}

type StatsMetricRow struct {
	TotalRuns        int64
	TotalTokens      int64
	PromptTokens     int64
	CompletionTokens int64
	AvgDurationMS    int64
}

type StatsListQuery struct {
	CurrentPage int
	PageSize    int
	DateStart   string
	DateEnd     string
	AgentID     *int64
	ProviderID  *int64
	UserID      *int64
}

type StatsByDateRow struct {
	Date string
	StatsMetricRow
}
type StatsByAgentRow struct {
	AgentID   int64
	AgentName string
	StatsMetricRow
}
type StatsByUserRow struct {
	Username string
	StatsMetricRow
}

type Repository interface {
	AgentOptions(ctx context.Context) ([]OptionRow, error)
	ProviderOptions(ctx context.Context) ([]OptionRow, error)
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Detail(ctx context.Context, id int64) (*RunDetailRow, error)
	Events(ctx context.Context, runID int64) ([]EventRow, error)
	ToolCalls(ctx context.Context, runID int64) ([]ToolCallRow, error)
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
