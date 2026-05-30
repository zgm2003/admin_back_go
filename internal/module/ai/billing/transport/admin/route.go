package admin

import (
	aibilling "admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aibilling.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-billing-rules")
	group.GET("/page-init", handler.PageInit)
	group.GET("", handler.List)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.DELETE("/:id", handler.Delete)
}
