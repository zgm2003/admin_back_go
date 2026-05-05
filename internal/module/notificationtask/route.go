package notificationtask

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/notification-tasks")
	v1.GET("/init", handler.Init)
	v1.GET("/status-count", handler.StatusCount)
	v1.GET("", handler.List)
	v1.POST("", handler.Create)
	v1.PATCH("/:id/cancel", handler.Cancel)
	v1.DELETE("/:id", handler.Delete)
}
