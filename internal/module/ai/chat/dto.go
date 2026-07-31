package aichat

import (
	"context"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/shared/apperror"
)

type ConversationReplyInput struct {
	CommandID          uint64
	LeaseOwner         string
	LeaseToken         uint64
	DeliveryContext    context.Context
	PrepareStartedAt   time.Time
	ConversationID     int64
	UserID             int64
	AgentID            int64
	UserMessageID      int64
	RequestID          string
	RequestIdentity    requestidentity.Input
	CommandAttempt     uint
	CommandMaxAttempts uint
}

type ConversationReplyResult struct {
	ConversationID     int64
	AssistantMessageID int64
	DeliveryStopped    bool
	Finalized          bool
}

type RunTimeoutInput struct {
	Limit        int
	StaleTimeout time.Duration
}

type RunTimeoutResult struct {
	Failed int64 `json:"failed"`
}

type CreateRunRecord struct {
	ConversationID        int64
	RequestID             string
	RequestFingerprint    [32]byte
	RequestIdentityStatus string
	RequestIdentityMarker string
	UserMessageID         int64
	UserID                int64
	AgentID               int64
	ProviderID            int64
	ModelID               string
	ModelDisplayName      string
	PricingSnapshotJSON   string
	StartedAt             time.Time
}

type CompleteRunRecord struct {
	RunID              int64
	AssistantMessageID int64
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	FinishedAt         time.Time
	DurationMS         uint
}

type FinishRunRecord struct {
	RunID      int64
	Status     string
	Message    string
	FinishedAt time.Time
	DurationMS uint
}
type AssistantPublication struct {
	CommandID      uint64
	ConversationID int64
	Owner          string
	Token          uint64
	Content        string
	Now            time.Time
}

type AssistantPublisher interface {
	PublishAssistant(context.Context, AssistantPublication) (int64, bool, error)
}

type DeliveryCommit struct {
	CommandID uint64
	Owner     string
	Token     uint64
	Delta     string
	Now       time.Time
}

type DeliveryCommitter interface {
	CommitDelivery(context.Context, DeliveryCommit) (deliverySeq uint32, committed bool, err error)
}

type ProviderAttemptState string

const (
	ProviderAttemptSucceeded      ProviderAttemptState = "succeeded"
	ProviderAttemptFailed         ProviderAttemptState = "failed"
	ProviderAttemptCanceled       ProviderAttemptState = "canceled"
	ProviderAttemptOutcomeUnknown ProviderAttemptState = "outcome_unknown"
)

type ProviderAttemptPrepareInput struct {
	RunID     int64
	CommandID uint64
	Owner     string
	Token     uint64
	Now       time.Time
}

type ProviderAttemptEvidenceKind string

const (
	ProviderAttemptEvidencePaid             ProviderAttemptEvidenceKind = "paid"
	ProviderAttemptEvidenceLegacyUnbillable ProviderAttemptEvidenceKind = "legacy_unbillable"
)

type ProviderAttemptRef struct {
	ID                  uint64
	RunID               int64
	AttemptNo           uint
	IdempotencyKey      string
	PreparedRequestJSON string
	QuoteJSON           string
	EvidenceKind        ProviderAttemptEvidenceKind
}

type ProviderAttemptMarkInput struct {
	RunID     int64
	AttemptID uint64
	CommandID uint64
	Owner     string
	Token     uint64
	Now       time.Time
}

type ProviderAttemptFinishInput struct {
	AttemptID           uint64
	RunID               int64
	CommandID           uint64
	Owner               string
	Token               uint64
	State               ProviderAttemptState
	ProviderRequestID   string
	ResponseSHA256      string
	ErrorCode           string
	DispatchState       string
	UsageJSON           string
	UsageStatus         string
	ResultCandidateJSON *string
	Now                 time.Time
}

type ProviderAttemptRecorder interface {
	PrepareProviderAttempt(context.Context, ProviderAttemptPrepareInput) (*ProviderAttemptRef, error)
	MarkProviderAttemptDispatched(context.Context, ProviderAttemptMarkInput) error
	FinishProviderAttempt(context.Context, ProviderAttemptFinishInput) error
}

type PaidChatAttemptInput struct {
	RunID              int64
	CommandID          uint64
	LeaseOwner         string
	LeaseToken         uint64
	RequestID          string
	RequestIdentity    requestidentity.Input
	DeliveryContext    context.Context
	PrepareStartedAt   time.Time
	CommandAttempt     uint
	CommandMaxAttempts uint
	Engine             infraai.Engine
	ChatInput          infraai.ChatInput
	Sink               infraai.EventSink
}

// PaidChatAttemptResult distinguishes a durable settlement from a provider
// result that still requires another tool or retry turn.
type PaidChatAttemptResult struct {
	ChatResult         *infraai.ChatResult
	Finalized          bool
	AssistantMessageID int64
}

// PaidChatAttemptExecutor owns the complete paid provider sequence. Its
// implementation must quote and reserve before persisting prepared evidence,
// then dispatch the exact persisted bytes through the process-local Gateway.
type PaidChatAttemptExecutor interface {
	ExecutePaidChatAttempt(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error)
}

type PreparedPaidAttemptProbe interface {
	HasPreparedPaidChatAttempt(context.Context, int64, uint64) (bool, error)
}

type PaidChatAttemptFinalizer interface {
	FinalizePaidChatAttempt(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error)
}

type PaidChatAttemptFailureFinalizer interface {
	FinalizePaidChatPreDispatchFailure(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error)
	FinalizePaidChatLocalFailure(context.Context, PaidChatAttemptInput) (*PaidChatAttemptResult, error)
}

type EngineConfig struct {
	EngineType    infraai.EngineType
	BaseURL       string
	APIKey        string
	FileInputMode string
	FileOpener    infraai.PreparedFileOpener
}

type EngineFactory interface {
	NewEngine(ctx context.Context, input EngineConfig) (infraai.Engine, error)
}

type AgentEngineConfig struct {
	AgentID                uint64
	AgentName              string
	ModelID                string
	ModelDisplayName       string
	SystemPrompt           string
	ScenesJSON             string
	ProviderID             uint64
	EngineType             string
	FileInputMode          string
	EngineBaseURL          string
	EngineAPIKeyEnc        string
	AgentStatus            int
	EngineStatus           int
	ProviderModelStatus    int
	OfficialModelID        string
	OfficialCatalogVersion string
	MappingStatus          officialmodel.MappingStatus
	BillingMultiplierPPM   int64
}

type MessageHistory struct {
	ID          int64
	Role        int
	ContentType string
	Content     string
	MetaJSON    *string
	CreatedAt   time.Time
}

type RuntimeTool = aitool.RuntimeTool

type ToolRuntime interface {
	ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeTool, *apperror.Error)
	Execute(ctx context.Context, input ToolExecuteInput) (*ToolExecuteResult, *apperror.Error)
}

type ToolExecuteInput = aitool.ExecuteInput

type ToolExecuteResult = aitool.ExecuteResult

type KnowledgeRuntime interface {
	RetrieveForRun(ctx context.Context, input KnowledgeRuntimeInput) (*KnowledgeContextResult, *apperror.Error)
}

type KnowledgeRuntimeInput struct {
	RunID          uint64
	AgentID        uint64
	ConversationID int64
	UserMessageID  int64
	Query          string
	StartedAt      time.Time
}

type KnowledgeContextResult struct {
	RetrievalID uint64
	Status      string
	Context     string
}

type Repository interface {
	ConversationForReply(ctx context.Context, id int64, userID int64) (*Conversation, error)
	AcceptedRunForReply(ctx context.Context, userID int64, requestID string) (*airun.Run, error)
	AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error)
	LatestMessages(ctx context.Context, conversationID int64, limit int) ([]MessageHistory, error)
	CreateRun(ctx context.Context, input CreateRunRecord) (int64, error)
	CompleteRun(ctx context.Context, input CompleteRunRecord) error
	FinishRun(ctx context.Context, input FinishRunRecord) error
	TimeoutRuns(ctx context.Context, limit int, staleBefore time.Time, message string) (int64, error)
}

type PreparedRecoveryRepository interface {
	ProviderForPreparedRecovery(context.Context, uint64) (*AgentEngineConfig, error)
}

type RunRecorder = airun.Recorder

type TextGeneration interface {
	ReplayAndWait(context.Context, aitext.ReplayInput) (*aitext.Result, bool, *apperror.Error)
	SubmitAndWait(context.Context, aitext.AcceptInput) (*aitext.Result, *apperror.Error)
}

type TextCompletionInput struct {
	Platform  string
	RequestID string
	UserID    int64
	AgentID   int64
	ModelID   string
	Message   string
}

type TextCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Content string `json:"content"`
}

type HTTPService interface {
	CompleteText(ctx context.Context, input TextCompletionInput) (*TextCompletionResponse, *apperror.Error)
}

type JobService interface {
	ExecuteConversationReply(ctx context.Context, input ConversationReplyInput) (*ConversationReplyResult, error)
	TimeoutRuns(ctx context.Context, input RunTimeoutInput) (*RunTimeoutResult, error)
}
