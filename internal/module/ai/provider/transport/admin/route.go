package admin

import (
	"net/http"

	infraai "admin_back_go/internal/infra/ai"
	aiprovidermodule "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aiprovidermodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-providers/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: aiprovidermodule.InitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-providers",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listRequest{},
			Response: aiprovidermodule.ListResponse{},
		},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-providers/model-options",
		Access: adminroute.Permission("ai_provider_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "preview_models",
			Title:   "拉取AI供应商模型",
		},
		Contract: &adminroute.HTTPContract{
			Request:  modelOptionsRequest{},
			Response: aiprovidermodule.ModelOptionsResponse{},
		},
	}, handler.PreviewModels)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-providers",
		Access: adminroute.Permission("ai_provider_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "create",
			Title:   "新增AI供应商",
		},
		Contract: &adminroute.HTTPContract{
			Request:  mutationRequest{},
			Response: adminroute.IDData{},
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-providers/:id",
		Access: adminroute.Permission("ai_provider_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "update",
			Title:   "编辑AI供应商",
		},
		Contract: &adminroute.HTTPContract{
			Request:  mutationRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/ai-providers/:id/status",
		Access: adminroute.Permission("ai_provider_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "change_status",
			Title:   "修改AI供应商状态",
		},
		Contract: &adminroute.HTTPContract{
			Request:  statusRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-providers/:id/model-options",
		Access: adminroute.Permission("ai_provider_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "preview_models",
			Title:   "拉取AI供应商模型",
		},
		Contract: &adminroute.HTTPContract{
			Response: aiprovidermodule.ModelOptionsResponse{},
		},
	}, handler.PreviewStoredModels)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-providers/:id/test",
		Access: adminroute.Permission("ai_provider_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "test",
			Title:   "测试AI供应商连接",
		},
		Contract: &adminroute.HTTPContract{
			Response: infraai.TestConnectionResult{},
		},
	}, handler.TestConnection)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/ai-providers/:id/sync-models",
		Access: adminroute.Permission("ai_provider_test"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "sync_models",
			Title:   "同步AI供应商模型",
		},
		Contract: &adminroute.HTTPContract{
			Response: aiprovidermodule.ModelOptionsResponse{},
		},
	}, handler.SyncModels)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-providers/:id/models",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: aiprovidermodule.ProviderModelsResponse{},
		},
	}, handler.ListProviderModels)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-providers/:id/models",
		Access: adminroute.Permission("ai_provider_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "update_models",
			Title:   "编辑AI供应商模型",
		},
		Contract: &adminroute.HTTPContract{
			Request:  updateModelsRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.UpdateProviderModels)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/ai-providers/:id",
		Access: adminroute.Permission("ai_provider_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "ai_provider",
			Action:  "delete",
			Title:   "删除AI供应商",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.Delete)
}
