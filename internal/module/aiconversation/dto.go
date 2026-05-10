package aiconversation

import (
	"context"

	"admin_back_go/internal/apperror"
)

type ListQuery struct {
	UserID   int64
	AgentID  *int64
	BeforeID int64
	Limit    int
}

type ListRow struct {
	Conversation Conversation
	AgentName    string
}

type ConversationItem struct {
	ID            int64  `json:"id"`
	AgentID       int64  `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	Title         string `json:"title"`
	LastMessageAt string `json:"last_message_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ConversationDetail struct {
	ID            int64  `json:"id"`
	AgentID       int64  `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	Title         string `json:"title"`
	LastMessageAt string `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ListResponse struct {
	List    []ConversationItem `json:"list"`
	NextID  int64              `json:"next_id"`
	HasMore bool               `json:"has_more"`
}

type CreateInput struct {
	AgentID int64
	Title   string
}

type UpdateInput struct {
	Title string
}

type CreateResponse struct {
	ID int64 `json:"id"`
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, bool, error)
	Get(ctx context.Context, id int64) (*Conversation, string, error)
	ActiveChatAgentExists(ctx context.Context, id int64) (bool, error)
	Create(ctx context.Context, row Conversation) (int64, error)
	UpdateTitle(ctx context.Context, id int64, userID int64, title string) error
	Delete(ctx context.Context, id int64, userID int64) error
}

type HTTPService interface {
	List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, userID int64, id int64) (*ConversationDetail, *apperror.Error)
	Create(ctx context.Context, userID int64, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, userID int64, id int64, input UpdateInput) *apperror.Error
	Delete(ctx context.Context, userID int64, id int64) *apperror.Error
}
