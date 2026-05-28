package admin

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	router.GET("/api/admin/v1/ai-conversations/:id/messages", handler.List)
	router.POST("/api/admin/v1/ai-conversations/:id/messages", handler.Send)
	router.POST("/api/admin/v1/ai-conversations/:id/messages/cancel", handler.Cancel)
}
