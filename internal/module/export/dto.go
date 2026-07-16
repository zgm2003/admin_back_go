package exporttask

import (
	"context"

	"admin_back_go/internal/shared/apperror"
)

type StatusCountQuery struct {
	UserID   int64
	Platform string
	Kind     string
	Title    string
	FileName string
}

type StatusCountItem struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Num   int64  `json:"num"`
}

type ListQuery struct {
	UserID      int64
	Platform    string
	Kind        string
	CurrentPage int
	PageSize    int
	BeforeID    int64
	Status      *int
	Title       string
	FileName    string
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List   []ListItem `json:"list"`
	Page   Page       `json:"page"`
	NextID int64      `json:"next_id"`
}

type ListItem struct {
	ID           int64   `json:"id"`
	Kind         string  `json:"kind"`
	KindText     string  `json:"kind_text"`
	Title        string  `json:"title"`
	FileName     *string `json:"file_name"`
	FileURL      *string `json:"file_url"`
	FileSizeText string  `json:"file_size_text"`
	RowCount     *int64  `json:"row_count"`
	Status       int     `json:"status"`
	StatusText   string  `json:"status_text"`
	ErrorMsg     *string `json:"error_msg"`
	ExpireAt     *string `json:"expire_at"`
	CreatedAt    string  `json:"created_at"`
}

type CreatePendingInput struct {
	UserID   int64
	Platform string
	Kind     string
	Title    string
}

type CreatePendingResponse struct {
	ID int64 `json:"id"`
}

type DeleteInput struct {
	UserID   int64
	Platform string
	IDs      []int64
}

type SuccessResult struct {
	FileName  string
	FileURL   string
	ObjectKey string
	FileSize  int64
	RowCount  int64
}

type HTTPService interface {
	StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Delete(ctx context.Context, input DeleteInput) *apperror.Error
}
