package aiprompt

import (
	"context"

	"admin_back_go/internal/apperror"
)

type InitResponse struct{}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      int64
	Title       string
	Category    string
	IsFavorite  *int
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
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Variables  []string `json:"variables"`
	IsFavorite int      `json:"is_favorite"`
	UseCount   int64    `json:"use_count"`
	Sort       int      `json:"sort"`
	CreatedAt  string   `json:"created_at"`
}

type DetailResponse struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Variables  []string `json:"variables"`
	IsFavorite int      `json:"is_favorite"`
	UseCount   int64    `json:"use_count"`
	Sort       int      `json:"sort"`
}

type CreateInput struct {
	Title     string
	Content   string
	Category  string
	Tags      []string
	Variables []string
}

type UpdateInput struct {
	Title     string
	Content   string
	Category  string
	Tags      []string
	Variables []string
}

type ToggleFavoriteResponse struct {
	IsFavorite int `json:"is_favorite"`
}

type UseResponse struct {
	Content string `json:"content"`
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, userID int64, id int64) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, userID int64, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, userID int64, id int64, input UpdateInput) *apperror.Error
	Delete(ctx context.Context, userID int64, id int64) *apperror.Error
	ToggleFavorite(ctx context.Context, userID int64, id int64) (*ToggleFavoriteResponse, *apperror.Error)
	Use(ctx context.Context, userID int64, id int64) (*UseResponse, *apperror.Error)
}
