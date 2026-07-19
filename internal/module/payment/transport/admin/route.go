package admin

import (
	"net/http"

	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/configs/page-init",
		Access: adminroute.Permission("payment_config_list"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: paymentmodule.ConfigPageInitResponse{},
		},
	}, handler.ConfigPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/configs",
		Access: adminroute.Permission("payment_config_list"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listConfigsRequest{},
			Response: paymentmodule.ConfigListResponse{},
		},
	}, handler.ListConfigs)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/payment/configs",
		Access: adminroute.Permission("payment_config_add"),
		Audit: adminroute.AuditDecision{
			Enabled:            true,
			Module:             "payment_config",
			Action:             "create",
			Title:              "新增支付配置",
			SkipRequestPayload: true,
		},
		Contract: &adminroute.HTTPContract{
			Request:  configMutationRequest{},
			Response: adminroute.IDData{},
		},
	}, handler.CreateConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/payment/configs/:id",
		Access: adminroute.Permission("payment_config_edit"),
		Audit: adminroute.AuditDecision{
			Enabled:            true,
			Module:             "payment_config",
			Action:             "update",
			Title:              "编辑支付配置",
			SkipRequestPayload: true,
		},
		Contract: &adminroute.HTTPContract{
			Request:  configMutationRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdateConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/payment/configs/:id/status",
		Access: adminroute.Permission("payment_config_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "payment_config",
			Action:  "change_status",
			Title:   "切换支付配置状态",
		},
		Contract: &adminroute.HTTPContract{
			Request:  changeConfigStatusRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.ChangeConfigStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/payment/configs/:id",
		Access: adminroute.Permission("payment_config_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "payment_config",
			Action:  "delete",
			Title:   "删除支付配置",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteConfig)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/payment/configs/:id/test",
		Access: adminroute.Permission("payment_config_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "payment_config",
			Action:  "test",
			Title:   "测试支付配置",
		},
		Contract: &adminroute.HTTPContract{
			Response: paymentmodule.ConfigTestResponse{},
		},
	}, handler.TestConfig)

	registerRechargeRoutes(routes, handler)

	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/payment/certificates",
		Access: adminroute.Permission("payment_config_upload_cert"),
		Audit: adminroute.AuditDecision{
			Enabled:            true,
			Module:             "payment_config",
			Action:             "upload_cert",
			Title:              "上传支付宝证书",
			SkipRequestPayload: true,
		},
		Contract: &adminroute.HTTPContract{
			Request:            certificateUploadRequest{},
			RequestContentType: "multipart/form-data",
			Response:           paymentmodule.CertificateUploadResponse{},
		},
	}, handler.UploadCertificate)
}

func registerRechargeRoutes(routes adminroute.Registrar, handler *Handler) {
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/recharges/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: paymentmodule.RechargePageInitResponse{},
		},
	}, handler.RechargePageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/recharges",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listRechargesRequest{},
			Response: paymentmodule.RechargeListResponse{},
		},
	}, handler.ListRecharges)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/recharges/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: paymentmodule.RechargeDetail{},
		},
	}, handler.GetRecharge)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/payment/recharges",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "payment_recharge",
			Action:  "add",
			Title:   "创建充值",
		},
		Contract: &adminroute.HTTPContract{
			Request:  createRechargeRequest{},
			Response: paymentmodule.RechargePayResponse{},
		},
	}, handler.CreateRecharge)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/payment/recharges/:id/pay",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled:             true,
			Module:              "payment_recharge",
			Action:              "pay",
			Title:               "继续支付",
			SkipResponsePayload: true,
		},
		Contract: &adminroute.HTTPContract{
			Response: paymentmodule.RechargePayResponse{},
		},
	}, handler.PayRecharge)
}
