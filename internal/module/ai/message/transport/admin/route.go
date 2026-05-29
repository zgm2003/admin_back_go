package admin

import (
	aimessagemodule "admin_back_go/internal/module/ai/message"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aimessagemodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	router.GET("/api/admin/v1/ai-conversations/:id/messages", handler.List)
	router.POST("/api/admin/v1/ai-conversations/:id/messages", handler.Send)
	router.POST("/api/admin/v1/ai-conversations/:id/messages/cancel", handler.Cancel)
}
