package server

import (
	aichatcanvas "admin_back_go/internal/module/ai/chat/transport/canvas"
	aiimagecanvas "admin_back_go/internal/module/ai/image/transport/canvas"
	canvastransport "admin_back_go/internal/module/canvas/transport/canvas"

	"github.com/gin-gonic/gin"
)

func registerCanvasRoutes(router *gin.Engine, deps Dependencies) {
	canvastransport.RegisterRoutes(router, deps.CanvasService)
	aiimagecanvas.RegisterRoutes(router, deps.AiImageService)
	aichatcanvas.RegisterRoutes(router, deps.AiChatService)
}
