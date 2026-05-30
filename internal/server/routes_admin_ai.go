package server

import (
	aiagentadmin "admin_back_go/internal/module/ai/agent/transport/admin"
	aibillingadmin "admin_back_go/internal/module/ai/billing/transport/admin"
	aichatadmin "admin_back_go/internal/module/ai/chat/transport/admin"
	aiconversationadmin "admin_back_go/internal/module/ai/conversation/transport/admin"
	aiimageadmin "admin_back_go/internal/module/ai/image/transport/admin"
	aiknowledgeadmin "admin_back_go/internal/module/ai/knowledge/transport/admin"
	aimessageadmin "admin_back_go/internal/module/ai/message/transport/admin"
	aiprovideradmin "admin_back_go/internal/module/ai/provider/transport/admin"
	airunadmin "admin_back_go/internal/module/ai/run/transport/admin"
	aitooladmin "admin_back_go/internal/module/ai/tool/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminAIRoutes(router *gin.Engine, deps Dependencies) {
	aiprovideradmin.Register(router, deps.AiProviderService)
	aibillingadmin.Register(router, deps.AiBillingService)
	aiagentadmin.Register(router, deps.AiAgentService)
	aitooladmin.Register(router, deps.AiToolService)
	aiknowledgeadmin.Register(router, deps.AiKnowledgeService)
	aiconversationadmin.Register(router, deps.AiConversationService)
	aimessageadmin.Register(router, deps.AiMessageService)
	airunadmin.Register(router, deps.AiRunService)
	aichatadmin.Register(router, deps.AiChatService)
	aiimageadmin.Register(router, deps.AiImageService)
}
