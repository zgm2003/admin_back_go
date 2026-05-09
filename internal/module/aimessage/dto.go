package aimessage

import (
	"context"

	"admin_back_go/internal/apperror"
)

type ListQuery struct {
	UserID         int64
	ConversationID int64
	BeforeID       int64
	Limit          int
}

type SendInput struct {
	ConversationID int64
	Content        string
	RequestID      string
}

type SendRecord struct {
	ConversationID int64
	Role           int
	ContentType    string
	Content        string
}

type ReplyPayload struct {
	ConversationID int64
	UserID         int64
	AgentID        int64
	UserMessageID  int64
	RequestID      string
}

type ReplyEnqueuer interface {
	EnqueueConversationReply(ctx context.Context, payload ReplyPayload) error
}

type MessageItem struct {
	ID          int64  `json:"id"`
	Role        int    `json:"role"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ListResponse struct {
	List    []MessageItem `json:"list"`
	NextID  int64         `json:"next_id"`
	HasMore bool          `json:"has_more"`
}

type SendResponse struct {
	ConversationID int64  `json:"conversation_id"`
	UserMessageID  int64  `json:"user_message_id"`
	RequestID      string `json:"request_id"`
}

type AgentRuntime struct {
	AgentID    int64
	Status     int
	ScenesJSON string
}

type Repository interface {
	Conversation(ctx context.Context, id int64) (*Conversation, error)
	AgentForConversation(ctx context.Context, conversationID int64, userID int64) (*AgentRuntime, error)
	List(ctx context.Context, query ListQuery) ([]Message, bool, error)
	InsertUserMessage(ctx context.Context, input SendRecord) (int64, error)
}

type HTTPService interface {
	List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error)
	Send(ctx context.Context, userID int64, input SendInput) (*SendResponse, *apperror.Error)
}
