package admin

import (
	"net/http"

	infraai "admin_back_go/internal/infra/ai"
	aiagentmodule "admin_back_go/internal/module/ai/agent"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aiagentmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-agents/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: aiagentmodule.InitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-agents",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listRequest{},
			Response: aiagentmodule.ListResponse{},
		},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-agents/options",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    optionRequest{},
			Response: aiagentmodule.AgentOptionsResponse{},
		},
	}, handler.Options)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-agents/provider-models/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: aiagentmodule.ProviderModelsResponse{},
		},
	}, handler.ProviderModels)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-agents/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: aiagentmodule.DetailResponse{},
		},
	}, handler.Detail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-agents",
		Access: adminroute.Permission("ai_agent_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_agent",
			Action:  "create",
			Title:   "新增AI智能体",
		},
		Contract: &adminroute.HTTPContract{
			Request:  mutationRequest{},
			Response: adminroute.IDData{},
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-agents/:id",
		Access: adminroute.Permission("ai_agent_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_agent",
			Action:  "update",
			Title:   "编辑AI智能体",
		},
		Contract: &adminroute.HTTPContract{
			Request:  mutationRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/ai-agents/:id/status",
		Access: adminroute.Permission("ai_agent_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_agent",
			Action:  "change_status",
			Title:   "修改AI智能体状态",
		},
		Contract: &adminroute.HTTPContract{
			Request:  statusRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-agents/:id/test",
		Access: adminroute.Permission("ai_agent_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_agent",
			Action:  "test",
			Title:   "测试AI智能体",
		},
		Contract: &adminroute.HTTPContract{
			Response: infraai.TestConnectionResult{},
		},
	}, handler.Test)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/ai-agents/:id",
		Access: adminroute.Permission("ai_agent_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_agent",
			Action:  "delete",
			Title:   "删除AI智能体",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.Delete)
}
