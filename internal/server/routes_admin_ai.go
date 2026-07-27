package server

import (
	aiagentadmin "admin_back_go/internal/module/ai/agent/transport/admin"
	aichatadmin "admin_back_go/internal/module/ai/chat/transport/admin"
	aiconversationadmin "admin_back_go/internal/module/ai/conversation/transport/admin"
	aiknowledgeadmin "admin_back_go/internal/module/ai/knowledge/transport/admin"
	aimessageadmin "admin_back_go/internal/module/ai/message/transport/admin"
	aimodelpricingadmin "admin_back_go/internal/module/ai/modelpricing/transport/admin"
	aiprovideradmin "admin_back_go/internal/module/ai/provider/transport/admin"
	airunadmin "admin_back_go/internal/module/ai/run/transport/admin"
	aitooladmin "admin_back_go/internal/module/ai/tool/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminAIRoutes(router *gin.Engine, deps Dependencies) {
	ai := deps.Admin.AI
	aiprovideradmin.Register(router, ai.Providers, deps.Core.RouteRegistry)
	aiagentadmin.Register(router, ai.Agents, deps.Core.RouteRegistry)
	aitooladmin.Register(router, ai.Tools, deps.Core.RouteRegistry)
	aiknowledgeadmin.Register(router, ai.Knowledge, deps.Core.RouteRegistry)
	aiconversationadmin.Register(router, ai.Conversations, deps.Core.RouteRegistry)
	aimessageadmin.Register(router, ai.Messages, deps.Core.RouteRegistry)
	aimodelpricingadmin.Register(router, ai.ModelPrices, deps.Core.RouteRegistry)
	airunadmin.Register(router, ai.Runs, deps.Core.RouteRegistry)
	aichatadmin.Register(router, ai.Chat)
}
