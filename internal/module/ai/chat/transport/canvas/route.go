package canvas

import (
	aichatmodule "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aichatmodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1/ai/chat")
	group.POST("/completions", handler.ChatCompletions)
}
