package aimessage

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	router.GET("/api/admin/v1/ai-conversations/:id/messages", handler.List)
	router.POST("/api/admin/v1/ai-conversations/:id/messages", handler.Send)
}
