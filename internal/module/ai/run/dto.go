package airun

import (
	"context"
	"encoding/json"
	"time"

	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
)

type JSONObject = json.RawMessage
type ContextPlanProjection = contextengine.ContextPlanProjection

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	StatusArr        []dict.Option[string] `json:"status_arr"`
	PlatformArr      []dict.Option[string] `json:"platform_arr"`
	ProviderArr      []dict.Option[int]    `json:"providerArr"`
	AgentArr         []dict.Option[int]    `json:"agentArr"`
	ModelArr         []ModelOption         `json:"model_arr"`
	BillingStatusArr []dict.Option[string] `json:"billing_status_arr"`
	BillingReasonArr []dict.Option[string] `json:"billing_reason_arr"`
}

type ModelOption struct {
	Label      string `json:"label"`
	Value      string `json:"value"`
	Historical bool   `json:"historical"`
}

type PageInitFilter struct {
	DateStart string
	DateEnd   string
}

type ListQuery struct {
	CurrentPage    int
	PageSize       int
	Platform       string
	Status         string
	UserID         *int64
	RequestID      string
	AgentID        *int64
	ProviderID     *int64
	ModelID        string
	BillingStatus  string
	BillingReason  string
	ErrorCode      string
	ToolCode       string
	RunAnomaly     string
	BillingAnomaly string
	UserFeedback   string
	AnomalyAsOf    string
	DateStart      string
	DateEnd        string
	StartAt        time.Time
	EndExclusive   time.Time
	GeneratedAt    time.Time
	StaleBefore    time.Time
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
	ProviderID        int64   `json:"provider_id"`
	ProviderName      string  `json:"provider_name"`
	Platform          string  `json:"platform"`
	InputSnapshot     string  `json:"input_snapshot"`
	ConversationID    *int64  `json:"conversation_id"`
	ConversationTitle string  `json:"conversation_title"`
	Status            string  `json:"status"`
	StatusName        string  `json:"status_name"`
	ModelID           string  `json:"model_id"`
	ModelDisplayName  string  `json:"model_display_name"`
	BillingStatus     string  `json:"billing_status"`
	BillingReason     string  `json:"billing_reason"`
	ErrorCode         string  `json:"error_code"`
	Liked             bool    `json:"liked"`
	LikedAt           *string `json:"liked_at"`
	PromptTokens      uint    `json:"prompt_tokens"`
	CompletionTokens  uint    `json:"completion_tokens"`
	TotalTokens       uint    `json:"total_tokens"`
	DurationMS        *uint   `json:"duration_ms"`
	DurationText      string  `json:"duration_text"`
	ErrorMessage      string  `json:"error_message"`
	CreatedAt         string  `json:"created_at"`
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
	ID                int64                   `json:"id"`
	RequestID         string                  `json:"request_id"`
	UserID            int64                   `json:"user_id"`
	Username          string                  `json:"username"`
	AgentID           int64                   `json:"agent_id"`
	AgentName         string                  `json:"agent_name"`
	ProviderID        int64                   `json:"provider_id"`
	ProviderName      string                  `json:"provider_name"`
	Platform          string                  `json:"platform"`
	InputSnapshot     string                  `json:"input_snapshot"`
	ConversationID    *int64                  `json:"conversation_id"`
	ConversationTitle string                  `json:"conversation_title"`
	Status            string                  `json:"status"`
	StatusName        string                  `json:"status_name"`
	ModelID           string                  `json:"model_id"`
	ModelDisplayName  string                  `json:"model_display_name"`
	PromptTokens      uint                    `json:"prompt_tokens"`
	CompletionTokens  uint                    `json:"completion_tokens"`
	TotalTokens       uint                    `json:"total_tokens"`
	DurationMS        *uint                   `json:"duration_ms"`
	DurationText      string                  `json:"duration_text"`
	ErrorCode         string                  `json:"error_code"`
	DiagnosticCodes   []string                `json:"diagnostic_codes"`
	ErrorMessage      string                  `json:"error_message"`
	BillingStatus     string                  `json:"billing_status"`
	BillingReason     string                  `json:"billing_reason"`
	HeldAmount        string                  `json:"held_amount"`
	ActualAmount      string                  `json:"actual_amount"`
	Pricing           *PricingDetail          `json:"pricing"`
	UsageItems        []UsageItemDetail       `json:"usage_items"`
	ProviderAttempts  []ProviderAttemptDetail `json:"provider_attempts"`
	Latency           LatencyBreakdown        `json:"latency"`
	RequestSummary    SafeRequestSummary      `json:"request_summary"`
	ContextPlan       *ContextPlanProjection  `json:"context_plan"`
	UserMessage       *MessageSummary         `json:"user_message"`
	AssistantMessage  *MessageSummary         `json:"assistant_message"`
	Events            []EventItem             `json:"events"`
	ToolCalls         []ToolCallItem          `json:"tool_calls"`
	Liked             bool                    `json:"liked"`
	LikedAt           *string                 `json:"liked_at"`
	StartedAt         string                  `json:"started_at"`
	FinishedAt        string                  `json:"finished_at"`
	CreatedAt         string                  `json:"created_at"`
	UpdatedAt         string                  `json:"updated_at"`
}

type LatencyBreakdown struct {
	AcceptMS        *int64 `json:"accept_ms"`
	QueueMS         *int64 `json:"queue_ms"`
	PrepareMS       *int64 `json:"prepare_ms"`
	COSHeadMS       *int64 `json:"cos_head_ms"`
	COSStreamMS     *int64 `json:"cos_stream_ms"`
	TTFTMS          *int64 `json:"ttft_ms"`
	ProviderTotalMS *int64 `json:"provider_total_ms"`
	SettlementMS    *int64 `json:"settlement_ms"`
	EndToEndMS      *int64 `json:"end_to_end_ms"`
	ClaimSource     string `json:"claim_source"`
}

type SafeRequestSummary struct {
	ProviderAttemptCount     int    `json:"provider_attempt_count"`
	ToolCallCount            int    `json:"tool_call_count"`
	PreparedRequestBytes     int    `json:"prepared_request_bytes"`
	MessageCount             *int   `json:"message_count"`
	AttachmentCount          int    `json:"attachment_count"`
	NativeFileCount          int    `json:"native_file_count"`
	NativeFileBytes          int64  `json:"native_file_bytes"`
	PreparedManifestBytes    int    `json:"prepared_manifest_bytes"`
	MaterializedRequestBytes int64  `json:"materialized_request_bytes"`
	APIProtocol              string `json:"api_protocol"`
}

type PricingDetail struct {
	Version           string              `json:"version"`
	CatalogVendor     string              `json:"catalog_vendor"`
	TransportEngine   string              `json:"transport_engine"`
	ModelID           string              `json:"model_id"`
	ResolvedAlias     string              `json:"resolved_alias"`
	BillingMultiplier string              `json:"billing_multiplier"`
	MaxOutputTokens   int                 `json:"max_output_tokens"`
	Rates             []PricingRateDetail `json:"rates"`
}

type PricingRateDetail struct {
	Category  string `json:"category"`
	TierKey   string `json:"tier_key"`
	Unit      string `json:"unit"`
	Price     string `json:"price"`
	UnitScale int64  `json:"unit_scale"`
}

type UsageItemDetail struct {
	AttemptNo uint   `json:"attempt_no"`
	Category  string `json:"category"`
	TierKey   string `json:"tier_key"`
	Quantity  int64  `json:"quantity"`
	Unit      string `json:"unit"`
	UnitPrice string `json:"unit_price"`
	UnitScale int64  `json:"unit_scale"`
	Amount    string `json:"amount"`
	Billable  bool   `json:"billable"`
}

type ProviderAttemptDetail struct {
	AttemptNo         uint    `json:"attempt_no"`
	State             string  `json:"state"`
	ProviderRequestID *string `json:"provider_request_id"`
	UsageStatus       string  `json:"usage_status"`
}

type OptionRow struct {
	ID   int64
	Name string
}

type HistoricalModelRow struct {
	ModelID          string
	ModelDisplayName string
}

type ListRow struct {
	ID                int64
	RequestID         string
	UserID            int64
	AgentID           int64
	AgentName         string
	ProviderID        int64
	ProviderName      string
	Platform          string
	InputSnapshot     string
	ConversationID    *int64
	ConversationTitle string
	Status            string
	ModelID           string
	ModelDisplayName  string
	BillingStatus     string
	BillingReason     string
	ErrorCode         string
	LikedAt           *time.Time
	PromptTokens      uint
	CompletionTokens  uint
	TotalTokens       uint
	DurationMS        *uint
	ErrorMessage      string
	CreatedAt         time.Time
}

type RunDetailRow struct {
	ID                  int64
	RequestID           string
	UserID              int64
	Username            string
	AgentID             int64
	AgentName           string
	ProviderID          int64
	ProviderName        string
	Platform            string
	InputSnapshot       string
	ConversationID      *int64
	ConversationTitle   string
	Status              string
	ModelID             string
	ModelDisplayName    string
	ErrorCode           string
	DiagnosticCodes     []string `gorm:"-"`
	PromptTokens        uint
	CompletionTokens    uint
	TotalTokens         uint
	DurationMS          *uint
	ErrorMessage        string
	PricingSnapshotJSON string
	BillingStatus       string
	BillingReason       string
	LikedAt             *time.Time
	RequestReceivedAt   *time.Time
	AcceptedAt          *time.Time
	ClaimedAt           *time.Time
	ClaimSource         string
	UserMessage         *MessageSummary `gorm:"-"`
	AssistantMessage    *MessageSummary `gorm:"-"`
	StartedAt           *time.Time
	FinishedAt          *time.Time
	SettledAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ChargeRow struct {
	ID          int64
	HeldUnits   int64
	ActualUnits int64
	Status      string
}

type UsageChargeItemRow struct {
	AttemptID      int64
	AttemptNo      uint
	AttemptState   string
	Category       string
	TierKey        string
	Quantity       int64
	Unit           string
	UnitPriceUnits int64
	UnitScale      int64
	AmountUnits    int64
}

type ProviderAttemptRow struct {
	ID                  int64
	AttemptNo           uint
	State               string
	ProviderRequestID   string
	UsageStatus         string
	UsageJSON           string
	PreparedRequestJSON string
	PrepareStartedAt    *time.Time
	DispatchedAt        *time.Time
	FirstDeltaAt        *time.Time
	FinishedAt          *time.Time
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

type Repository interface {
	AgentOptions(ctx context.Context) ([]OptionRow, error)
	ProviderOptions(ctx context.Context) ([]OptionRow, error)
	HistoricalModelOptions(ctx context.Context, startAt, endExclusive time.Time) ([]HistoricalModelRow, error)
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Detail(ctx context.Context, id int64) (*RunDetailRow, error)
	BillingDetail(ctx context.Context, runID int64) (*ChargeRow, []UsageChargeItemRow, []ProviderAttemptRow, error)
	Events(ctx context.Context, runID int64) ([]EventRow, error)
	ToolCalls(ctx context.Context, runID int64) ([]ToolCallRow, error)
	ContextPlan(ctx context.Context, runID int64) (*contextengine.ContextPlan, error)
	Dashboard(ctx context.Context, query DashboardQuery) (DashboardRepositoryResult, error)
}

type HTTPService interface {
	PageInit(ctx context.Context, filter PageInitFilter) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error)
	Dashboard(ctx context.Context, filter DashboardFilter) (*DashboardResponse, *apperror.Error)
}

type FeedbackHTTPService interface {
	SetUserFeedback(ctx context.Context, userID int64, id int64, liked bool) (*FeedbackResponse, *apperror.Error)
}
