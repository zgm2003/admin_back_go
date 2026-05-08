package aimessage

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	router.GET("/api/admin/v1/ai-conversations/:id/messages", handler.List)
	group := router.Group("/api/admin/v1/ai-messages")
	group.PATCH("/:id/content", handler.EditContent)
	group.PATCH("/:id/feedback", handler.Feedback)
	group.DELETE("/:id", handler.Delete)
	group.DELETE("", handler.BatchDelete)
}
