package server

import (
	"admin_back_go/internal/module/aiagent"
	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/aiconversation"
	"admin_back_go/internal/module/aiimage"
	"admin_back_go/internal/module/aiknowledge"
	"admin_back_go/internal/module/aimessage"
	"admin_back_go/internal/module/aiprovider"
	"admin_back_go/internal/module/airun"
	"admin_back_go/internal/module/aitool"

	"github.com/gin-gonic/gin"
)

func registerAdminAIRoutes(router *gin.Engine, deps Dependencies) {
	aiprovider.RegisterRoutes(router, deps.AiProviderService)
	aiagent.RegisterRoutes(router, deps.AiAgentService)
	aiimage.RegisterRoutes(router, deps.AiImageService)
	aiknowledge.RegisterRoutes(router, deps.AiKnowledgeService)
	aiconversation.RegisterRoutes(router, deps.AiConversationService)
	aimessage.RegisterRoutes(router, deps.AiMessageService)
	airun.RegisterRoutes(router, deps.AiRunService)
	aichat.RegisterRoutes(router, deps.AiChatService)
	aitool.RegisterRoutes(router, deps.AiToolService)
}
