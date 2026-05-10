package ai

import "context"

type EngineType string

const (
	EngineTypeOpenAI EngineType = "openai"
)

type TestConnectionInput struct {
	EngineType EngineType
	BaseURL    string
	APIKey     string
	TimeoutMs  int
}

type TestConnectionResult struct {
	OK        bool
	Status    string
	LatencyMs int
	Message   string
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
	EngineConversationID string
	EngineMessageID      string
	EngineTaskID         string
	Answer               string
	ToolCalls            []ToolCall
	PromptTokens         int
	CompletionTokens     int
	TotalTokens          int
	Cost                 float64
	LatencyMs            int
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
