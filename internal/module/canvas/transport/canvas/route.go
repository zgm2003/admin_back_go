package canvas

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1")
	group.GET("/settings", handler.Settings)
	group.GET("/prompts", handler.Prompts)
	group.GET("/assets", handler.Assets)
	group.POST("/ai/chat/completions", handler.ChatCompletions)
	group.POST("/ai/images/generations", handler.ImageGenerations)
	group.POST("/ai/images/edits", handler.ImageEdits)
	group.GET("/ai/images/:id", handler.ImageStatus)
	group.POST("/ai/videos", handler.VideoGenerations)
	group.GET("/ai/videos/:id", handler.VideoStatus)
	group.GET("/ai/videos/:id/content", handler.VideoContent)
}
