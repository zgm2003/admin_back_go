package aimodel

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	AIDriverArr     []dict.Option[string] `json:"ai_driver_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Driver      string
	Status      *int
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
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	DriverName string `json:"driver_name"`
	ModelCode  string `json:"model_code"`
	Endpoint   string `json:"endpoint"`
	APIKeyHint string `json:"api_key_hint"`
	Status     int    `json:"status"`
	StatusName string `json:"status_name"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type CreateInput struct {
	Name      string
	Driver    string
	ModelCode string
	Endpoint  string
	APIKey    string
	Status    int
}

type UpdateInput struct {
	Name      string
	Driver    string
	ModelCode string
	Endpoint  string
	APIKey    string
	Status    int
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
}
