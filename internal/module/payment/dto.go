package payment

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ChannelInitResponse struct {
	Dict ChannelInitDict `json:"dict"`
}

type ChannelInitDict struct {
	ProviderArr     []dict.Option[string] `json:"provider_arr"`
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	PayMethodArr    []dict.Option[string] `json:"pay_method_arr"`
	YesNoArr        []dict.Option[int]    `json:"yes_no_arr"`
}

type ChannelListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Provider    string
	Status      int
}

type ChannelListResponse struct {
	List []ChannelListItem `json:"list"`
	Page Page              `json:"page"`
}

type ChannelListItem struct {
	ID                 int64    `json:"id"`
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	Provider           string   `json:"provider"`
	ProviderText       string   `json:"provider_text"`
	SupportedMethods   []string `json:"supported_methods"`
	SupportedText      string   `json:"supported_methods_text"`
	AppID              string   `json:"app_id"`
	MerchantID         string   `json:"merchant_id"`
	NotifyURL          string   `json:"notify_url"`
	ReturnURL          string   `json:"return_url"`
	PrivateKeyHint     string   `json:"private_key_hint"`
	AppCertPath        string   `json:"app_cert_path"`
	AlipayCertPath     string   `json:"alipay_cert_path"`
	AlipayRootCertPath string   `json:"alipay_root_cert_path"`
	IsSandbox          int      `json:"is_sandbox"`
	Status             int      `json:"status"`
	StatusText         string   `json:"status_text"`
	Remark             string   `json:"remark"`
	CreatedAt          string   `json:"created_at"`
	UpdatedAt          string   `json:"updated_at"`
}

type ChannelMutationInput struct {
	Code               string
	Name               string
	Provider           string
	SupportedMethods   []string
	AppID              string
	MerchantID         string
	NotifyURL          string
	ReturnURL          string
	PrivateKey         string
	AppCertPath        string
	AlipayCertPath     string
	AlipayRootCertPath string
	IsSandbox          int
	Status             int
	Remark             string
}

type OrderListQuery struct {
	CurrentPage int
	PageSize    int
	OrderNo     string
	UserID      int64
	Status      int
	StartDate   string
	EndDate     string
}

type OrderListResponse struct {
	List []OrderListItem `json:"list"`
	Page Page            `json:"page"`
}

type OrderListItem struct {
	ID          int64  `json:"id"`
	OrderNo     string `json:"order_no"`
	UserID      int64  `json:"user_id"`
	ChannelID   int64  `json:"channel_id"`
	Provider    string `json:"provider"`
	PayMethod   string `json:"pay_method"`
	Subject     string `json:"subject"`
	AmountCents int64  `json:"amount_cents"`
	Status      int    `json:"status"`
	StatusText  string `json:"status_text"`
	OutTradeNo  string `json:"out_trade_no"`
	TradeNo     string `json:"trade_no"`
	PaidAt      string `json:"paid_at"`
	ExpiredAt   string `json:"expired_at"`
	ClosedAt    string `json:"closed_at"`
	CreatedAt   string `json:"created_at"`
}

type CreateOrderInput struct {
	UserID       int64
	ChannelID    int64
	PayMethod    string
	Subject      string
	AmountCents  int64
	ReturnURL    string
	BusinessType string
	BusinessRef  string
	ClientIP     string
}

type NumberGenerator interface {
	Next(ctx context.Context, prefix string) (string, error)
}

type CreateOrderResponse struct {
	OrderNo     string `json:"order_no"`
	AmountCents int64  `json:"amount_cents"`
	ExpiredAt   string `json:"expired_at"`
}

type PayOrderResponse struct {
	OrderNo    string         `json:"order_no"`
	OutTradeNo string         `json:"out_trade_no"`
	PayMethod  string         `json:"pay_method"`
	PayURL     string         `json:"pay_url"`
	PayData    map[string]any `json:"pay_data"`
}

type ResultResponse struct {
	OrderNo    string `json:"order_no"`
	Status     int    `json:"status"`
	StatusText string `json:"status_text"`
	TradeNo    string `json:"trade_no"`
	PaidAt     string `json:"paid_at"`
}

type EventListQuery struct {
	CurrentPage   int
	PageSize      int
	OrderNo       string
	OutTradeNo    string
	EventType     string
	ProcessStatus int
}

type EventListResponse struct {
	List []EventListItem `json:"list"`
	Page Page            `json:"page"`
}

type EventListItem struct {
	ID            int64  `json:"id"`
	OrderNo       string `json:"order_no"`
	OutTradeNo    string `json:"out_trade_no"`
	EventType     string `json:"event_type"`
	EventTypeText string `json:"event_type_text"`
	Provider      string `json:"provider"`
	ProcessStatus int    `json:"process_status"`
	ProcessText   string `json:"process_status_text"`
	ErrorMessage  string `json:"error_message"`
	CreatedAt     string `json:"created_at"`
}

type OrderDetailResponse struct {
	Order OrderListItem `json:"order"`
}

type EventDetailResponse struct {
	Event        EventListItem  `json:"event"`
	RequestData  map[string]any `json:"request_data"`
	ResponseData map[string]any `json:"response_data"`
}

type NotifyInput struct {
	Form map[string]string
	IP   string
}

type CloseExpiredInput struct {
	Limit int
	Now   time.Time
}

type SyncPendingInput struct {
	Limit int
	Now   time.Time
}

type JobResult struct {
	Scanned  int
	Closed   int
	Paid     int
	Deferred int
	Skipped  int
}

type HTTPService interface {
	ChannelInit(ctx context.Context) (*ChannelInitResponse, *apperror.Error)
	ListChannels(ctx context.Context, query ChannelListQuery) (*ChannelListResponse, *apperror.Error)
	CreateChannel(ctx context.Context, input ChannelMutationInput) (int64, *apperror.Error)
	UpdateChannel(ctx context.Context, id int64, input ChannelMutationInput) *apperror.Error
	ChangeChannelStatus(ctx context.Context, id int64, status int) *apperror.Error
	DeleteChannel(ctx context.Context, id int64) *apperror.Error
	OrderInit(ctx context.Context) (*ChannelInitResponse, *apperror.Error)
	ListOrders(ctx context.Context, query OrderListQuery) (*OrderListResponse, *apperror.Error)
	GetAdminOrder(ctx context.Context, orderNo string) (*OrderDetailResponse, *apperror.Error)
	GetOrderResult(ctx context.Context, userID int64, orderNo string) (*ResultResponse, *apperror.Error)
	CreateOrder(ctx context.Context, input CreateOrderInput) (*CreateOrderResponse, *apperror.Error)
	PayOrder(ctx context.Context, userID int64, orderNo string, returnURL string) (*PayOrderResponse, *apperror.Error)
	CancelOrder(ctx context.Context, userID int64, orderNo string) *apperror.Error
	CloseAdminOrder(ctx context.Context, orderNo string) *apperror.Error
	ListEvents(ctx context.Context, query EventListQuery) (*EventListResponse, *apperror.Error)
	GetEvent(ctx context.Context, id int64) (*EventDetailResponse, *apperror.Error)
	HandleAlipayNotify(ctx context.Context, input NotifyInput) (string, *apperror.Error)
}
