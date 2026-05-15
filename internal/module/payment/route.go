package payment

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	configs := router.Group("/api/admin/v1/payment/configs")
	configs.GET("/page-init", handler.ConfigInit)
	configs.GET("", handler.ListConfigs)
	configs.POST("", handler.CreateConfig)
	configs.PUT("/:id", handler.UpdateConfig)
	configs.PATCH("/:id/status", handler.ChangeConfigStatus)
	configs.DELETE("/:id", handler.DeleteConfig)
	configs.POST("/:id/test", handler.TestConfig)

	orders := router.Group("/api/admin/v1/payment/orders")
	orders.GET("/page-init", handler.OrderInit)
	orders.GET("", handler.ListOrders)
	orders.GET("/:id", handler.GetOrder)
	orders.POST("", handler.CreateOrder)
	orders.POST("/:id/pay", handler.PayOrder)
	orders.POST("/:id/sync", handler.SyncOrder)
	orders.PATCH("/:id/close", handler.CloseOrder)

	router.POST("/api/admin/v1/payment/certificates", handler.UploadCertificate)
}
