package replycommand

import (
	"strings"
	"time"

	"admin_back_go/internal/module/ai/requestidentity"
)

type State string

const (
	StatePending        State = "pending"
	StateClaimed        State = "claimed"
	StateRunning        State = "running"
	StateSucceeded      State = "succeeded"
	StateFailed         State = "failed"
	StateCanceled       State = "canceled"
	StateOutcomeUnknown State = "outcome_unknown"
	StateTimedOut       State = "timed_out"
)

type ClaimSource string

const (
	ClaimSourceWake     ClaimSource = "wake"
	ClaimSourcePoll     ClaimSource = "poll"
	ClaimSourceRecovery ClaimSource = "recovery"
)

const (
	// ErrCodeFinalizationRetry marks a command that must settle persisted facts
	// without constructing another provider request.
	ErrCodeFinalizationRetry = "ai.finalization_retry"
	FinalizationRetryMarker  = "finalization_retry"
)

func (command Command) IsGenericFinalizationRetry() bool {
	return strings.TrimSpace(command.LastErrorCode) == ErrCodeFinalizationRetry &&
		strings.TrimSpace(command.LastErrorMessage) == FinalizationRetryMarker
}

func (command Command) RequiresFinalizationOnly() bool {
	if command.CancelRequestedAt != nil {
		return true
	}
	code := strings.TrimSpace(command.LastErrorCode)
	marker := strings.TrimSpace(command.LastErrorMessage)
	switch code {
	case ErrCodeFinalizationRetry:
		return command.IsGenericFinalizationRetry()
	case "ai.provider_failed":
		return marker == "provider_failed"
	case "ai.local_failed":
		return marker == "local_failure"
	case "ai.provider_pre_dispatch_failed":
		return marker == "pre_dispatch_failed"
	case "ai.billing.insufficient_balance":
		return marker == "initial_insufficient" || marker == "continuation_topup_insufficient"
	default:
		return false
	}
}

type Command struct {
	ID                    uint64      `gorm:"column:id;primaryKey"`
	RequestID             string      `gorm:"column:request_id"`
	RequestFingerprint    []byte      `gorm:"column:request_fingerprint"`
	RequestIdentityStatus string      `gorm:"column:request_identity_status"`
	RequestIdentityMarker string      `gorm:"column:request_identity_marker"`
	IdempotencyKey        string      `gorm:"column:idempotency_key"`
	Platform              string      `gorm:"column:platform"`
	UserID                int64       `gorm:"column:user_id"`
	ConversationID        int64       `gorm:"column:conversation_id"`
	UserMessageID         int64       `gorm:"column:user_message_id"`
	AssistantMessageID    *int64      `gorm:"column:assistant_message_id"`
	RequestReceivedAt     *time.Time  `gorm:"column:request_received_at"`
	AcceptedAt            *time.Time  `gorm:"column:accepted_at"`
	ClaimedAt             *time.Time  `gorm:"column:claimed_at"`
	ClaimSource           ClaimSource `gorm:"column:claim_source"`
	State                 State       `gorm:"column:state"`
	AttemptCount          uint        `gorm:"column:attempt_count"`
	MaxAttempts           uint        `gorm:"column:max_attempts"`
	LeaseOwner            *string     `gorm:"column:lease_owner"`
	LeaseToken            uint64      `gorm:"column:lease_token"`
	LeaseExpiresAt        *time.Time  `gorm:"column:lease_expires_at"`
	NextAttemptAt         time.Time   `gorm:"column:next_attempt_at"`
	CancelRequestedAt     *time.Time  `gorm:"column:cancel_requested_at"`
	OutcomeUnknownAt      *time.Time  `gorm:"column:outcome_unknown_at"`
	LastErrorCode         string      `gorm:"column:last_error_code"`
	LastErrorMessage      string      `gorm:"column:last_error_message"`
	StartedAt             *time.Time  `gorm:"column:started_at"`
	FinishedAt            *time.Time  `gorm:"column:finished_at"`
	CreatedAt             time.Time   `gorm:"column:created_at"`
	UpdatedAt             time.Time   `gorm:"column:updated_at"`
}

type Renewal struct {
	Alive           bool
	CancelRequested bool
}

func (Command) TableName() string { return "ai_reply_commands" }

type CreateReplyInput struct {
	ConversationID        int64
	UserID                int64
	AgentID               int64
	ProviderID            int64
	ModelID               string
	ModelDisplayName      string
	RequestID             string
	RequestReceivedAt     time.Time
	Content               string
	MetaJSON              *string
	InputSnapshot         string
	PricingSnapshotJSON   string
	EffectiveMaxTokens    int64
	RequestFingerprint    [32]byte
	RequestIdentityStatus requestidentity.IdentityStatus
	RequestIdentityMarker string
}

type CreateReplyResult struct {
	UserMessageID int64
	CommandID     uint64
	RunID         int64
	ChargeID      int64
	RequestID     string
	State         State
}
