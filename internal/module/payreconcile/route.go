package payreconcile

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/pay-reconcile-tasks")
	group.GET("/page-init", handler.Init)
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.PATCH("/:id/retry", handler.Retry)
	group.GET("/:id/files/:type", handler.File)
}
