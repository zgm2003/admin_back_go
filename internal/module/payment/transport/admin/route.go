package admin

import (
	"net/http"

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
	}, handler.ConfigPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/configs",
		Access: adminroute.Permission("payment_config_list"),
		Audit:  adminroute.NoAudit("read-only"),
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
	}, handler.UploadCertificate)
}

func registerRechargeRoutes(routes adminroute.Registrar, handler *Handler) {
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/recharges/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.RechargePageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/recharges",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.ListRecharges)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/recharges/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
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
	}, handler.PayRecharge)
}
