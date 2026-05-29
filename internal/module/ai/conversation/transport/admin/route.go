package admin

import (
	aiconversationmodule "admin_back_go/internal/module/ai/conversation"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aiconversationmodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-conversations")
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.DELETE("/:id", handler.Delete)
}
