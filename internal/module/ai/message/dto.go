package aimessage

import (
	"context"
	"time"

	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/uploadpolicy"
)

type ListQuery struct {
	UserID         int64
	ConversationID int64
	BeforeID       int64
	Limit          int
}

type SendInput struct {
	ConversationID    int64
	Content           string
	RequestID         string
	RequestReceivedAt time.Time
	Attachments       []Attachment
	RuntimeParams     map[string]float64
}

type CancelInput struct {
	ConversationID int64
	RequestID      string
	DeliveredSeq   uint32
}

const (
	DeliveryStateCompleted = replycommand.DeliveryStateCompleted
	DeliveryStateStopped   = replycommand.DeliveryStateStopped
)

type EditInput struct {
	UserID                  int64
	ConversationID          int64
	MessageID               int64
	Content                 string
	RequestID               string
	Attachments             *[]Attachment
	ValidatedAttachments    []Attachment
	SourceAttachmentsSHA256 [32]byte
	SourceRuntimeSHA256     [32]byte
	UploadRuleToken         uploadpolicy.ConsistencyToken
}

type RegenerateInput struct {
	UserID                  int64
	ConversationID          int64
	AssistantMessageID      int64
	RequestID               string
	ValidatedAttachments    []Attachment
	SourceAttachmentsSHA256 [32]byte
	SourceRuntimeSHA256     [32]byte
	UploadRuleToken         uploadpolicy.ConsistencyToken
}

type DeleteInput struct {
	UserID         int64
	ConversationID int64
	IDs            []int64
}

type Attachment struct {
	Type      string `json:"type"`
	ObjectKey string `json:"object_key"`
	MIMEType  string `json:"mime_type"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`
}

type ReplyWaker interface {
	WakeReply(ctx context.Context, commandID uint64) error
}

type CancelPublisher interface {
	PublishCancel(ctx context.Context, commandID uint64) error
}

type MessageMetaAttachment struct {
	Type      string `json:"type"`
	ObjectKey string `json:"object_key,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
}

type MessageRuntimeParams struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxHistory  *int     `json:"max_history,omitempty"`
}

type MessageContext = contextengine.MessageContext

type MessageMeta struct {
	Attachments   []MessageMetaAttachment `json:"attachments,omitempty"`
	RuntimeParams *MessageRuntimeParams   `json:"runtime_params,omitempty"`
}

type MessageItem struct {
	ID                int64           `json:"id"`
	Role              int             `json:"role"`
	ContentType       string          `json:"content_type"`
	Content           string          `json:"content"`
	MetaJSON          *MessageMeta    `json:"meta_json,omitempty"`
	PairedMessageID   *int64          `json:"paired_message_id"`
	RunID             *int64          `json:"run_id"`
	Liked             bool            `json:"liked"`
	DeliveryState     *string         `json:"delivery_state"`
	SettlementPending bool            `json:"settlement_pending"`
	Context           *MessageContext `json:"context"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

type MessageProjection struct {
	Message
	PairedMessageID   *int64 `gorm:"column:paired_message_id"`
	RunID             *int64 `gorm:"column:run_id"`
	Liked             bool   `gorm:"column:liked"`
	SettlementPending bool   `gorm:"column:settlement_pending"`
}

type ListResponse struct {
	List    []MessageItem `json:"list"`
	NextID  int64         `json:"next_id"`
	HasMore bool          `json:"has_more"`
}

type SendResponse struct {
	ConversationID int64              `json:"conversation_id"`
	UserMessageID  int64              `json:"user_message_id"`
	CommandID      uint64             `json:"command_id"`
	RequestID      string             `json:"request_id"`
	State          replycommand.State `json:"state"`
}

type CancelResponse struct {
	ConversationID     int64  `json:"conversation_id"`
	RequestID          string `json:"request_id"`
	Status             string `json:"status"`
	AssistantMessageID *int64 `json:"assistant_message_id"`
	SettlementPending  bool   `json:"settlement_pending"`
}

type DeleteResponse struct {
	DeletedIDs []int64 `json:"deleted_ids"`
}

type HistoryAccepted struct {
	Reply    replycommand.CreateReplyResult
	Replayed bool
}

type HistoryActionPreparation struct {
	Runtime                 AgentRuntime
	SourceAttachments       []Attachment
	SourceAttachmentsSHA256 [32]byte
}

type HistoryPrepareInput struct {
	Operation       string
	UserID          int64
	ConversationID  int64
	SourceMessageID int64
}

type AgentRuntime struct {
	AgentID                int64
	ProviderID             int64
	ModelID                string
	ModelDisplayName       string
	EngineType             string
	APIProtocol            string
	ProviderModelStatus    int
	OfficialModelID        string
	OfficialCatalogVersion string
	MappingStatus          officialmodel.MappingStatus
	BillingMultiplierPPM   int64
	Status                 int
	ScenesJSON             string
}

type Repository interface {
	Conversation(ctx context.Context, id int64) (*Conversation, error)
	AgentForConversation(ctx context.Context, conversationID int64, userID int64) (*AgentRuntime, error)
	List(ctx context.Context, query ListQuery) ([]MessageProjection, bool, error)
	CreateReply(ctx context.Context, input replycommand.CreateReplyInput) (replycommand.CreateReplyResult, error)
	RequestCancel(ctx context.Context, input replycommand.RequestCancelInput) (replycommand.RequestCancelResult, error)
}

type ContextPlanRepository interface {
	ContextPlans(context.Context, []uint64) (map[uint64]contextengine.ContextPlan, error)
}

type HistoryRepository interface {
	PrepareAction(context.Context, HistoryPrepareInput) (HistoryActionPreparation, error)
	Revise(context.Context, EditInput) (HistoryAccepted, error)
	Regenerate(context.Context, RegenerateInput) (HistoryAccepted, error)
	DeleteMessages(context.Context, DeleteInput) ([]int64, error)
}

type HTTPService interface {
	List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error)
	Send(ctx context.Context, userID int64, input SendInput) (*SendResponse, *apperror.Error)
	Cancel(ctx context.Context, userID int64, input CancelInput) (*CancelResponse, *apperror.Error)
}

type HistoryHTTPService interface {
	Revise(ctx context.Context, userID int64, input EditInput) (*SendResponse, *apperror.Error)
	Regenerate(ctx context.Context, userID int64, input RegenerateInput) (*SendResponse, *apperror.Error)
	DeleteMessages(ctx context.Context, userID int64, input DeleteInput) (*DeleteResponse, *apperror.Error)
}
