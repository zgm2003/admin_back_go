package paychannel

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	ChannelArr      []dict.Option[int]    `json:"channel_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	PayMethodArr    []dict.Option[string] `json:"pay_method_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Channel     *int
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
	ID                   int64    `json:"id"`
	Name                 string   `json:"name"`
	Channel              int      `json:"channel"`
	ChannelName          string   `json:"channel_name"`
	SupportedMethods     []string `json:"supported_methods"`
	SupportedMethodsText string   `json:"supported_methods_text"`
	MchID                string   `json:"mch_id"`
	AppID                string   `json:"app_id"`
	NotifyURL            string   `json:"notify_url"`
	AppPrivateKeyHint    string   `json:"app_private_key_hint"`
	PublicCertPath       string   `json:"public_cert_path"`
	PlatformCertPath     string   `json:"platform_cert_path"`
	RootCertPath         string   `json:"root_cert_path"`
	Sort                 int      `json:"sort"`
	IsSandbox            int      `json:"is_sandbox"`
	IsSandboxText        string   `json:"is_sandbox_text"`
	Status               int      `json:"status"`
	StatusName           string   `json:"status_name"`
	Remark               string   `json:"remark"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type CreateInput struct {
	Name             string
	Channel          int
	SupportedMethods []string
	MchID            string
	AppID            string
	NotifyURL        string
	AppPrivateKey    string
	PublicCertPath   string
	PlatformCertPath string
	RootCertPath     string
	Sort             int
	IsSandbox        int
	Status           int
	Remark           string
}

type UpdateInput struct {
	Name             string
	Channel          int
	SupportedMethods []string
	MchID            string
	AppID            string
	NotifyURL        string
	AppPrivateKey    string
	PublicCertPath   string
	PlatformCertPath string
	RootCertPath     string
	Sort             int
	IsSandbox        int
	Status           int
	Remark           string
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
}
