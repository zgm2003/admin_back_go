package aimessage

import (
	"context"

	"admin_back_go/internal/apperror"
)

type JSONObject map[string]any

type ListQuery struct {
	ConversationID int64
	CurrentPage    int
	PageSize       int
	Role           *int
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
	ID             int64      `json:"id"`
	ConversationID int64      `json:"conversation_id"`
	Role           int        `json:"role"`
	Content        string     `json:"content"`
	MetaJSON       JSONObject `json:"meta_json"`
	CreatedAt      string     `json:"created_at"`
}

type EditContentResponse struct {
	DeletedCount int64 `json:"deleted_count"`
}

type DeleteResponse struct {
	Affected int64 `json:"affected"`
}

type Repository interface {
	Conversation(ctx context.Context, id int64) (*Conversation, error)
	Message(ctx context.Context, id int64) (*Message, error)
	List(ctx context.Context, query ListQuery) ([]Message, int64, error)
	UpdateContent(ctx context.Context, id int64, content string) error
	DeleteAfterMessage(ctx context.Context, conversationID int64, messageID int64) (int64, error)
	UpdateMeta(ctx context.Context, id int64, metaJSON *string) error
	DeleteMessages(ctx context.Context, ids []int64, userID int64) (int64, error)
}

type HTTPService interface {
	List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error)
	EditContent(ctx context.Context, userID int64, id int64, content string) (*EditContentResponse, *apperror.Error)
	Feedback(ctx context.Context, userID int64, id int64, feedback *int) *apperror.Error
	Delete(ctx context.Context, userID int64, ids []int64) (*DeleteResponse, *apperror.Error)
}
