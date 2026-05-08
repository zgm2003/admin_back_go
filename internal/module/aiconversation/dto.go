package aiconversation

import (
	"context"

	"admin_back_go/internal/apperror"
)

type ListQuery struct {
	UserID      int64
	CurrentPage int
	PageSize    int
	Status      *int
	AppID       *int64
	AgentID     *int64 // legacy alias for AppID during one Vue migration pass
	Title       string
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
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	AppID         int64  `json:"app_id"`
	AppName       string `json:"app_name"`
	AgentID       int64  `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	Title         string `json:"title"`
	LastMessageAt string `json:"last_message_at"`
	Status        int    `json:"status"`
	StatusName    string `json:"status_name"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type DetailResponse struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	AppID         int64  `json:"app_id"`
	AppName       string `json:"app_name"`
	AgentID       int64  `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	Title         string `json:"title"`
	LastMessageAt string `json:"last_message_at"`
	Status        int    `json:"status"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type MutationInput struct {
	AppID   int64
	AgentID int64 // legacy alias for AppID during one Vue migration pass
	Title   string
	Status  int
}

type ListRow struct {
	Conversation Conversation
	AppName      string
}

type Repository interface {
	List(ctx context.Context, query ListQuery) ([]ListRow, int64, error)
	Get(ctx context.Context, id int64) (*Conversation, error)
	AppName(ctx context.Context, id int64) (string, error)
	ActiveAppExists(ctx context.Context, id int64) (bool, error)
	Create(ctx context.Context, row Conversation) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id int64, status int) error
	Delete(ctx context.Context, id int64) error
}

type HTTPService interface {
	List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, userID int64, id int64) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, userID int64, input MutationInput) (int64, *apperror.Error)
	Update(ctx context.Context, userID int64, id int64, input MutationInput) *apperror.Error
	ChangeStatus(ctx context.Context, userID int64, id int64, status int) *apperror.Error
	Delete(ctx context.Context, userID int64, id int64) *apperror.Error
}
