package authplatform

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/auth-platforms")
	v1.GET("/init", handler.Init)
	v1.GET("", handler.List)
	v1.POST("", handler.Create)
	v1.PUT("/:id", handler.Update)
	v1.PATCH("/:id/status", handler.ChangeStatus)
	v1.DELETE("/:id", handler.DeleteOne)
	v1.DELETE("", handler.DeleteBatch)
}
