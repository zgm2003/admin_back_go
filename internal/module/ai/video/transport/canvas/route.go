package canvas

import (
	aivideomodule "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aivideomodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1/ai/videos")
	group.POST("", handler.VideoGenerations)
	group.GET("/:id", handler.VideoStatus)
	group.GET("/:id/content", handler.VideoContent)
}
