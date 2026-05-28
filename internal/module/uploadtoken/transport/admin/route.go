package admin

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	adminGroup := router.Group("/api/admin/v1/upload-tokens")
	adminGroup.POST("", handler.Create)
}
