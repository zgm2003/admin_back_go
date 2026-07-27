package admin

import (
	"net/http"

	aiconversationmodule "admin_back_go/internal/module/ai/conversation"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aiconversationmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-conversations",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-conversations/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Detail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-conversations",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_conversation",
			Action:  "create",
			Title:   "新增AI会话",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-conversations/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service AI conversation state"),
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-conversations/:id/read-cursor",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service AI conversation read state"),
	}, handler.AdvanceReadCursor)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/ai-conversations/:id",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_conversation",
			Action:  "delete",
			Title:   "删除AI会话",
		},
	}, handler.Delete)
}
