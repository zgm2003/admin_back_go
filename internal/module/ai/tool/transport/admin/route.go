package admin

import (
	"net/http"

	aitoolmodule "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aitoolmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-tools/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-tools/generate/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.GeneratePageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-tools",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-tools/generate-draft",
		Access: adminroute.Permission("ai_tool_generate"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_tool",
			Action:  "generate_draft",
			Title:   "AI生成工具草稿",
		},
	}, handler.GenerateDraft)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-tools",
		Access: adminroute.Permission("ai_tool_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_tool",
			Action:  "create",
			Title:   "新增AI工具",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-tools/:id",
		Access: adminroute.Permission("ai_tool_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_tool",
			Action:  "update",
			Title:   "编辑AI工具",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/ai-tools/:id/status",
		Access: adminroute.Permission("ai_tool_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_tool",
			Action:  "change_status",
			Title:   "修改AI工具状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/ai-tools/:id",
		Access: adminroute.Permission("ai_tool_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_tool",
			Action:  "delete",
			Title:   "删除AI工具",
		},
	}, handler.Delete)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-agents/:id/tools",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.AgentTools)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-agents/:id/tools",
		Access: adminroute.Permission("ai_agent_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_agent_tool",
			Action:  "update_binding",
			Title:   "更新智能体工具绑定",
		},
	}, handler.UpdateAgentTools)
}
