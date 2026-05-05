package notification

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type Identity struct {
	UserID   int64
	Platform string
}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	NotificationTypeArr       []dict.Option[int] `json:"notification_type_arr"`
	NotificationLevelArr      []dict.Option[int] `json:"notification_level_arr"`
	NotificationReadStatusArr []dict.Option[int] `json:"notification_read_status_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      int64
	Platform    string
	Keyword     string
	Type        *int
	Level       *int
	IsRead      *int
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
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      int    `json:"type"`
	TypeText  string `json:"type_text"`
	Level     int    `json:"level"`
	LevelText string `json:"level_text"`
	Link      string `json:"link"`
	IsRead    int    `json:"is_read"`
	CreatedAt string `json:"created_at"`
}

type UnreadCountResponse struct {
	Count int64 `json:"count"`
}

type MarkReadInput struct {
	UserID   int64
	Platform string
	IDs      []int64
}

type DeleteInput struct {
	UserID   int64
	Platform string
	IDs      []int64
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	UnreadCount(ctx context.Context, identity Identity) (*UnreadCountResponse, *apperror.Error)
	MarkRead(ctx context.Context, identity Identity, ids []int64) *apperror.Error
	Delete(ctx context.Context, identity Identity, ids []int64) *apperror.Error
}
