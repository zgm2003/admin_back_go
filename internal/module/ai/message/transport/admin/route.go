package admin

import (
	"net/http"

	aimessagemodule "admin_back_go/internal/module/ai/message"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aimessagemodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-conversations/:id/messages",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-conversations/:id/messages",
		Access: adminroute.Authenticated(),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_message",
			Action:  "send",
			Title:   "发送AI消息",
		},
	}, handler.Send)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-conversations/:id/messages/cancel",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service AI run cancellation"),
	}, handler.Cancel)
}
