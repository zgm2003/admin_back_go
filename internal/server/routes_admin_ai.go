package server

import (
	aiagentadmin "admin_back_go/internal/module/aiagent/transport/admin"
	aichatadmin "admin_back_go/internal/module/aichat/transport/admin"
	aiconversationadmin "admin_back_go/internal/module/aiconversation/transport/admin"
	aiimageadmin "admin_back_go/internal/module/aiimage/transport/admin"
	aiknowledgeadmin "admin_back_go/internal/module/aiknowledge/transport/admin"
	aimessageadmin "admin_back_go/internal/module/aimessage/transport/admin"
	aiprovideradmin "admin_back_go/internal/module/aiprovider/transport/admin"
	airunadmin "admin_back_go/internal/module/airun/transport/admin"
	aitooladmin "admin_back_go/internal/module/aitool/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminAIRoutes(router *gin.Engine, deps Dependencies) {
	aiprovideradmin.Register(router, deps.AiProviderService)
	aiagentadmin.Register(router, deps.AiAgentService)
	aitooladmin.Register(router, deps.AiToolService)
	aiknowledgeadmin.Register(router, deps.AiKnowledgeService)
	aiconversationadmin.Register(router, deps.AiConversationService)
	aimessageadmin.Register(router, deps.AiMessageService)
	airunadmin.Register(router, deps.AiRunService)
	aichatadmin.Register(router, deps.AiChatService)
	aiimageadmin.Register(router, deps.AiImageService)
}
