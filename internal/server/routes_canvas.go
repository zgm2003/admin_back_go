package server

import (
	aiassetcanvas "admin_back_go/internal/module/ai/asset/transport/canvas"
	aiaudiocanvas "admin_back_go/internal/module/ai/audio/transport/canvas"
	aichatcanvas "admin_back_go/internal/module/ai/chat/transport/canvas"
	aiimagecanvas "admin_back_go/internal/module/ai/image/transport/canvas"
	aipromptcanvas "admin_back_go/internal/module/ai/prompt/transport/canvas"
	aivideocanvas "admin_back_go/internal/module/ai/video/transport/canvas"
	canvastransport "admin_back_go/internal/module/canvas/transport/canvas"

	"github.com/gin-gonic/gin"
)

func registerCanvasRoutes(router *gin.Engine, deps Dependencies) {
	retired := deps.Retired
	canvastransport.RegisterRoutes(router, retired.Canvas)
	aiassetcanvas.RegisterRoutes(router, retired.AIAssets)
	aipromptcanvas.RegisterRoutes(router, retired.AIPrompt)
	aiimagecanvas.RegisterRoutes(router, retired.AIImages)
	aichatcanvas.RegisterRoutes(router, retired.AIChat)
	aivideocanvas.RegisterRoutes(router, retired.AIVideo)
	aiaudiocanvas.RegisterRoutes(router, retired.AIAudio)
}
