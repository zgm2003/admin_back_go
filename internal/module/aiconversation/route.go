package aiconversation

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-conversations")
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.POST("", handler.Create)
	group.DELETE("/:id", handler.Delete)
}
