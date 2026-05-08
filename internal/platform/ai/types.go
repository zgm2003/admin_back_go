package ai

import "context"

type EngineType string

const (
	EngineTypeDify    EngineType = "dify"
	EngineTypeEino    EngineType = "eino"
	EngineTypeDirect  EngineType = "direct"
	EngineTypeRAGFlow EngineType = "ragflow"
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

type ChatInput struct {
	AppID                uint64
	RunID                uint64
	UserID               uint64
	UserKey              string
	Content              string
	ConversationEngineID string
	Inputs               map[string]any
}

type ChatResult struct {
	EngineConversationID string
	EngineMessageID      string
	EngineTaskID         string
	Answer               string
	PromptTokens         int
	CompletionTokens     int
	TotalTokens          int
	Cost                 float64
	LatencyMs            int
}

type StopChatInput struct {
	EngineTaskID string
	UserKey      string
}

type KnowledgeSyncInput struct {
	DatasetID string
	Document  KnowledgeDocument
}

type KnowledgeDocument struct {
	Name      string
	Text      string
	SourceRef string
}

type KnowledgeSyncResult struct {
	EngineDatasetID  string
	EngineDocumentID string
	EngineBatch      string
	IndexingStatus   string
}

type KnowledgeStatusInput struct {
	DatasetID  string
	DocumentID string
}

type KnowledgeStatusResult struct {
	IndexingStatus string
	ErrorMessage   string
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
	StopChat(ctx context.Context, input StopChatInput) error
	SyncKnowledge(ctx context.Context, input KnowledgeSyncInput) (*KnowledgeSyncResult, error)
	KnowledgeStatus(ctx context.Context, input KnowledgeStatusInput) (*KnowledgeStatusResult, error)
}
