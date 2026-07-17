package replycommand

import "time"

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

type Command struct {
	ID                 uint64     `gorm:"column:id;primaryKey"`
	RequestID          string     `gorm:"column:request_id"`
	IdempotencyKey     string     `gorm:"column:idempotency_key"`
	Platform           string     `gorm:"column:platform"`
	UserID             int64      `gorm:"column:user_id"`
	ConversationID     int64      `gorm:"column:conversation_id"`
	UserMessageID      int64      `gorm:"column:user_message_id"`
	AssistantMessageID *int64     `gorm:"column:assistant_message_id"`
	State              State      `gorm:"column:state"`
	AttemptCount       uint       `gorm:"column:attempt_count"`
	MaxAttempts        uint       `gorm:"column:max_attempts"`
	LeaseOwner         *string    `gorm:"column:lease_owner"`
	LeaseToken         uint64     `gorm:"column:lease_token"`
	LeaseExpiresAt     *time.Time `gorm:"column:lease_expires_at"`
	NextAttemptAt      time.Time  `gorm:"column:next_attempt_at"`
	CancelRequestedAt  *time.Time `gorm:"column:cancel_requested_at"`
	OutcomeUnknownAt   *time.Time `gorm:"column:outcome_unknown_at"`
	LastErrorCode      string     `gorm:"column:last_error_code"`
	LastErrorMessage   string     `gorm:"column:last_error_message"`
	StartedAt          *time.Time `gorm:"column:started_at"`
	FinishedAt         *time.Time `gorm:"column:finished_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (Command) TableName() string { return "ai_reply_commands" }

type CreateReplyInput struct {
	ConversationID int64
	UserID         int64
	RequestID      string
	Content        string
	MetaJSON       *string
}

type CreateReplyResult struct {
	UserMessageID int64
	CommandID     uint64
	RequestID     string
	State         State
}
