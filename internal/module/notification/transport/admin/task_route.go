package admin

import (
	notificationtaskmodule "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterTaskRoutes(router *gin.Engine, service notificationtaskmodule.HTTPService) {
	validate.MustRegister()
	handler := NewTaskHandler(service)

	v1 := router.Group("/api/admin/v1/notification-tasks")
	v1.GET("/init", handler.Init)
	v1.GET("/status-count", handler.StatusCount)
	v1.GET("", handler.List)
	v1.POST("", handler.Create)
	v1.PATCH("/:id/cancel", handler.Cancel)
	v1.DELETE("/:id", handler.Delete)
}
