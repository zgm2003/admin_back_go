package clientversion

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type Page struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	Total       int64 `json:"total"`
	TotalPage   int64 `json:"total_page"`
}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	ClientVersionPlatformArr []dict.Option[string] `json:"client_version_platform_arr"`
	CommonYesNoArr           []dict.Option[int]    `json:"common_yes_no_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Platform    string
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID              int64  `json:"id"`
	Version         string `json:"version"`
	Notes           string `json:"notes"`
	FileURL         string `json:"file_url"`
	Signature       string `json:"signature"`
	Platform        string `json:"platform"`
	PlatformName    string `json:"platform_name"`
	FileSize        int64  `json:"file_size"`
	FileSizeText    string `json:"file_size_text"`
	IsLatest        int    `json:"is_latest"`
	IsLatestName    string `json:"is_latest_name"`
	ForceUpdate     int    `json:"force_update"`
	ForceUpdateName string `json:"force_update_name"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type CreateInput struct {
	Version     string
	Notes       string
	FileURL     string
	Signature   string
	Platform    string
	FileSize    int64
	ForceUpdate int
}

type UpdateInput struct {
	Version     string
	Notes       string
	FileURL     string
	Signature   string
	Platform    string
	FileSize    int64
	ForceUpdate int
}

type CurrentCheckQuery struct {
	Version  string
	Platform string
}

type CurrentCheckResponse struct {
	ForceUpdate bool `json:"force_update"`
}

type ManifestPlatform struct {
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

type ManifestPayload struct {
	Version   string                      `json:"version"`
	Notes     string                      `json:"notes"`
	PubDate   string                      `json:"pub_date"`
	Platforms map[string]ManifestPlatform `json:"platforms"`
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	SetLatest(ctx context.Context, id int64) *apperror.Error
	ForceUpdate(ctx context.Context, id int64, forceUpdate int) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
	UpdateJSON(ctx context.Context, platform string) (any, *apperror.Error)
	CurrentCheck(ctx context.Context, query CurrentCheckQuery) (*CurrentCheckResponse, *apperror.Error)
}
