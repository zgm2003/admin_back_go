package admin

import (
	"net/http"

	"admin_back_go/internal/module/ai/modelpricing"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service modelpricing.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/ai-model-prices/page-init",
		Access: adminroute.Permission("ai_model_pricing_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Response: modelpricing.PageInitResponse{}},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/ai-model-prices",
		Access: adminroute.Permission("ai_model_pricing_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Query: listRequest{}, Response: modelpricing.ListResponse{}},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/ai-model-prices/:model_id",
		Access: adminroute.Permission("ai_model_pricing_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Response: modelpricing.ModelPriceDTO{}},
	}, handler.Detail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut, Path: "/api/admin/v1/ai-model-prices/:model_id",
		Access:   adminroute.Permission("ai_model_pricing_edit"),
		Audit:    adminroute.AuditDecision{Enabled: true, Module: "ai_model_pricing", Action: "update", Title: "编辑模型定价"},
		Contract: &adminroute.HTTPContract{Request: updateRequest{}, Response: modelpricing.MutationResponse{}},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete, Path: "/api/admin/v1/ai-model-prices/:model_id/override",
		Access:   adminroute.Permission("ai_model_pricing_edit"),
		Audit:    adminroute.AuditDecision{Enabled: true, Module: "ai_model_pricing", Action: "restore_official", Title: "恢复模型官方定价"},
		Contract: &adminroute.HTTPContract{Query: restoreRequest{}, Response: modelpricing.MutationResponse{}},
	}, handler.RestoreOfficial)
}
