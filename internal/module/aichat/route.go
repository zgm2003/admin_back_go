package aichat

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-chat")
	group.POST("/runs", handler.CreateRun)
	group.GET("/runs/:run_id/events", handler.Events)
	group.POST("/messages", handler.SendMessage)
	group.POST("/runs/:run_id/cancel", handler.Cancel)
}
