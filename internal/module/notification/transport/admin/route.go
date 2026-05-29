package admin

import (
	notificationmodule "admin_back_go/internal/module/notification"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service notificationmodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/notifications")
	v1.GET("/init", handler.Init)
	v1.GET("/unread-count", handler.UnreadCount)
	v1.GET("", handler.List)
	v1.PATCH("/:id/read", handler.MarkOneRead)
	v1.PATCH("/read", handler.MarkRead)
	v1.DELETE("/:id", handler.DeleteOne)
	v1.DELETE("", handler.DeleteBatch)
}
