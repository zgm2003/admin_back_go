package admin

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-prompts")
	group.GET("/page-init", handler.PageInit)
	group.GET("", handler.List)
	group.POST("", handler.Create)
	group.GET("/:id", handler.Detail)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.DELETE("/:id", handler.DeleteOne)
	group.DELETE("", handler.DeleteBatch)
}
