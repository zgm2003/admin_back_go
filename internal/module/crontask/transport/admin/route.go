package admin

import (
	crontaskmodule "admin_back_go/internal/module/crontask"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service crontaskmodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/cron-tasks")
	group.GET("/page-init", handler.PageInit)
	group.GET("", handler.List)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.DELETE("/:id", handler.DeleteOne)
	group.DELETE("", handler.DeleteBatch)
	group.GET("/:id/logs", handler.Logs)
}
