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

	router.POST("/api/admin/v1/payment/certificates", handler.UploadCertificate)
}
