package aiprompt

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/ai-prompts")
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.DELETE("/:id", handler.Delete)
	group.PATCH("/:id/favorite", handler.ToggleFavorite)
	group.POST("/:id/use", handler.Use)
}
