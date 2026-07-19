package ai

import "context"

type EngineType string

const (
	EngineTypeOpenAI EngineType = "openai"
)

const (
	UsageStatusReported    = "reported"
	UsageStatusUnavailable = "unavailable"
)

type TestConnectionInput struct {
	EngineType EngineType
	BaseURL    string
	APIKey     string
	TimeoutMs  int
}

type TestConnectionResult struct {
	OK        bool   `json:"ok"`
	Status    string `json:"status"`
	LatencyMs int    `json:"latency_ms"`
	Message   string `json:"message"`
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ToolOutput struct {
	CallID string
	Name   string
	Output string
}

type ChatInput struct {
	AttemptID            uint64
	IdempotencyKey       string
	AgentID              uint64
	RunID                uint64
	UserID               uint64
	UserKey              string
	Content              string
	ConversationEngineID string
	Inputs               map[string]any
	Tools                []ToolDefinition
	ToolCalls            []ToolCall
	ToolOutputs          []ToolOutput
}

type ChatResult struct {
	ProviderRequestID    string
	EngineConversationID string
	EngineMessageID      string
	EngineTaskID         string
	Answer               string
	ToolCalls            []ToolCall
	PromptTokens         int
	CompletionTokens     int
	TotalTokens          int
	UsageStatus          string
	Cost                 float64
	LatencyMs            int
}

type VideoInput struct {
	Model           string
	Prompt          string
	DurationSeconds int
	Size            string
	ResolutionName  string
	GenerateAudio   *bool
	Watermark       *bool
}

type VideoTask struct {
	ID           string
	Status       string
	ErrorMessage string
	RawResponse  map[string]any
}

type VideoEngine interface {
	CreateVideo(ctx context.Context, input VideoInput) (*VideoTask, error)
	GetVideo(ctx context.Context, taskID string) (*VideoTask, error)
	DownloadVideo(ctx context.Context, taskID string) ([]byte, string, error)
}

type AudioInput struct {
	Model          string
	Prompt         string
	Voice          string
	ResponseFormat string
	Speed          *float64
	Instructions   string
}

type AudioResult struct {
	Body        []byte
	ContentType string
}

type AudioEngine interface {
	GenerateAudio(ctx context.Context, input AudioInput) (*AudioResult, error)
}

type Event struct {
	Type      string
	DeltaText string
	Payload   map[string]any
}

type EventSink interface {
	Emit(ctx context.Context, event Event) error
}

type Engine interface {
	TestConnection(ctx context.Context, input TestConnectionInput) (*TestConnectionResult, error)
	StreamChat(ctx context.Context, input ChatInput, sink EventSink) (*ChatResult, error)
}
