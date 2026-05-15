package payment

import (
	"context"
	"io"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ConfigInitResponse struct {
	Dict ConfigInitDict `json:"dict"`
}

type ConfigInitDict struct {
	ProviderArr        []dict.Option[string] `json:"provider_arr"`
	EnvironmentArr     []dict.Option[string] `json:"environment_arr"`
	CommonStatusArr    []dict.Option[int]    `json:"common_status_arr"`
	EnabledMethodArr   []dict.Option[string] `json:"enabled_method_arr"`
	CertificateTypeArr []dict.Option[string] `json:"certificate_type_arr"`
}

type ConfigListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Provider    string
	Environment string
	Status      int
}

type ConfigListResponse struct {
	List []ConfigListItem `json:"list"`
	Page Page             `json:"page"`
}

type ConfigListItem struct {
	ID                 int64    `json:"id"`
	Provider           string   `json:"provider"`
	ProviderText       string   `json:"provider_text"`
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	AppID              string   `json:"app_id"`
	PrivateKeyHint     string   `json:"private_key_hint"`
	AppCertPath        string   `json:"app_cert_path"`
	PlatformCertPath   string   `json:"platform_cert_path"`
	RootCertPath       string   `json:"root_cert_path"`
	NotifyURL          string   `json:"notify_url"`
	Environment        string   `json:"environment"`
	EnvironmentText    string   `json:"environment_text"`
	EnabledMethods     []string `json:"enabled_methods"`
	EnabledMethodsText string   `json:"enabled_methods_text"`
	Status             int      `json:"status"`
	StatusText         string   `json:"status_text"`
	Remark             string   `json:"remark"`
	CreatedAt          string   `json:"created_at"`
	UpdatedAt          string   `json:"updated_at"`
}

type ConfigMutationInput struct {
	ID               int64
	Provider         string
	Code             string
	Name             string
	AppID            string
	AppPrivateKey    string
	AppCertPath      string
	PlatformCertPath string
	RootCertPath     string
	NotifyURL        string
	Environment      string
	EnabledMethods   []string
	Status           int
	Remark           string
}

type CertificateUploadInput struct {
	ConfigCode string
	CertType   string
	FileName   string
	Size       int64
	Reader     io.Reader
}

type CertificateUploadResponse struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
}

type ConfigTestResponse struct {
	OK      bool     `json:"ok"`
	Checks  []string `json:"checks"`
	Message string   `json:"message"`
}

type HTTPService interface {
	ConfigInit(ctx context.Context) (*ConfigInitResponse, *apperror.Error)
	ListConfigs(ctx context.Context, query ConfigListQuery) (*ConfigListResponse, *apperror.Error)
	CreateConfig(ctx context.Context, input ConfigMutationInput) (int64, *apperror.Error)
	UpdateConfig(ctx context.Context, id int64, input ConfigMutationInput) *apperror.Error
	ChangeConfigStatus(ctx context.Context, id int64, status int) *apperror.Error
	DeleteConfig(ctx context.Context, id int64) *apperror.Error
	UploadCertificate(ctx context.Context, input CertificateUploadInput) (*CertificateUploadResponse, *apperror.Error)
	TestConfig(ctx context.Context, id int64) (*ConfigTestResponse, *apperror.Error)
	OrderInit(ctx context.Context) (*OrderInitResponse, *apperror.Error)
	ListOrders(ctx context.Context, query OrderListQuery) (*OrderListResponse, *apperror.Error)
	GetOrder(ctx context.Context, id int64) (*OrderDetail, *apperror.Error)
	CreateOrder(ctx context.Context, input OrderCreateInput) (*OrderCreateResponse, *apperror.Error)
	PayOrder(ctx context.Context, id int64) (*OrderPayResponse, *apperror.Error)
	SyncOrder(ctx context.Context, id int64) (*OrderStatusResponse, *apperror.Error)
	CloseOrder(ctx context.Context, id int64) (*OrderStatusResponse, *apperror.Error)
}
