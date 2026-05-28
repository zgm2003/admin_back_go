package admin

import (
	"context"

	"admin_back_go/internal/apperror"
	paymentmodule "admin_back_go/internal/module/payment"
)

type (
	ConfigInitResponse        = paymentmodule.ConfigInitResponse
	ConfigListQuery           = paymentmodule.ConfigListQuery
	ConfigListResponse        = paymentmodule.ConfigListResponse
	ConfigMutationInput       = paymentmodule.ConfigMutationInput
	CertificateUploadInput    = paymentmodule.CertificateUploadInput
	CertificateUploadResponse = paymentmodule.CertificateUploadResponse
	ConfigTestResponse        = paymentmodule.ConfigTestResponse
	OrderInitResponse         = paymentmodule.OrderInitResponse
	OrderListQuery            = paymentmodule.OrderListQuery
	OrderListResponse         = paymentmodule.OrderListResponse
	OrderDetail               = paymentmodule.OrderDetail
	OrderCreateInput          = paymentmodule.OrderCreateInput
	OrderCreateResponse       = paymentmodule.OrderCreateResponse
	OrderPayResponse          = paymentmodule.OrderPayResponse
	OrderStatusResponse       = paymentmodule.OrderStatusResponse
	RechargeInitResponse      = paymentmodule.RechargeInitResponse
	RechargeListQuery         = paymentmodule.RechargeListQuery
	RechargeListResponse      = paymentmodule.RechargeListResponse
	RechargeDetail            = paymentmodule.RechargeDetail
	RechargeCreateInput       = paymentmodule.RechargeCreateInput
	RechargePayResponse       = paymentmodule.RechargePayResponse
	RechargeStatusResponse    = paymentmodule.RechargeStatusResponse
)

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
	RechargeInit(ctx context.Context, userID int64) (*RechargeInitResponse, *apperror.Error)
	ListRecharges(ctx context.Context, query RechargeListQuery) (*RechargeListResponse, *apperror.Error)
	GetRecharge(ctx context.Context, userID int64, id int64) (*RechargeDetail, *apperror.Error)
	CreateRecharge(ctx context.Context, input RechargeCreateInput) (*RechargePayResponse, *apperror.Error)
	PayRecharge(ctx context.Context, userID int64, id int64) (*RechargePayResponse, *apperror.Error)
	SyncRecharge(ctx context.Context, userID int64, id int64) (*RechargeStatusResponse, *apperror.Error)
	CloseRecharge(ctx context.Context, userID int64, id int64) (*RechargeStatusResponse, *apperror.Error)
}
