package admin

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	configs := router.Group("/api/admin/v1/payment/configs")
	configs.GET("/page-init", handler.ConfigPageInit)
	configs.GET("", handler.ListConfigs)
	configs.POST("", handler.CreateConfig)
	configs.PUT("/:id", handler.UpdateConfig)
	configs.PATCH("/:id/status", handler.ChangeConfigStatus)
	configs.DELETE("/:id", handler.DeleteConfig)
	configs.POST("/:id/test", handler.TestConfig)

	registerRechargeRoutes(router, "/api/admin/v1/payment/recharges", handler, true)

	router.POST("/api/admin/v1/payment/certificates", handler.UploadCertificate)
}

func RegisterRechargeRoutes(router *gin.Engine, prefix string, service HTTPService) {
	validate.MustRegister()
	registerRechargeRoutes(router, prefix, NewHandler(service), false)
}

func registerRechargeRoutes(router *gin.Engine, prefix string, handler *Handler, includeDetail bool) {
	recharges := router.Group(prefix)
	recharges.GET("/page-init", handler.RechargePageInit)
	recharges.GET("", handler.ListRecharges)
	if includeDetail {
		recharges.GET("/:id", handler.GetRecharge)
	}
	recharges.POST("", handler.CreateRecharge)
	recharges.POST("/:id/pay", handler.PayRecharge)
}
