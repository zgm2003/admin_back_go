package admin

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-prompts/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-prompts",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-prompts",
		Access: adminroute.Permission("ai_prompt_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_prompt",
			Action:  "create",
			Title:   "新增AI提示词",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-prompts/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Detail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-prompts/:id",
		Access: adminroute.Permission("ai_prompt_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_prompt",
			Action:  "update",
			Title:   "编辑AI提示词",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/ai-prompts/:id/status",
		Access: adminroute.Permission("ai_prompt_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_prompt",
			Action:  "change_status",
			Title:   "修改AI提示词状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/ai-prompts/:id",
		Access: adminroute.Permission("ai_prompt_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_prompt",
			Action:  "delete",
			Title:   "删除AI提示词",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/ai-prompts",
		Access: adminroute.Permission("ai_prompt_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_prompt",
			Action:  "delete_batch",
			Title:   "批量删除AI提示词",
		},
	}, handler.DeleteBatch)
}
