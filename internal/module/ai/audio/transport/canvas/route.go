package canvas

import (
	aiaudiomodule "admin_back_go/internal/module/ai/audio"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aiaudiomodule.HTTPService) {
	if router == nil {
		return
	}
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1/ai/audios")
	group.POST("", handler.AudioGenerations)
}
