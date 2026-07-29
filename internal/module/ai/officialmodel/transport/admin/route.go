package admin

import (
	"net/http"

	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service officialmodel.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/ai-official-models/page-init",
		Access: adminroute.Permission("ai_official_model_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Response: officialmodel.PageInitResponse{}},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/ai-official-models",
		Access: adminroute.Permission("ai_official_model_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Query: listRequest{}, Response: officialmodel.ListResponse{}},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/ai-official-models/:model_id",
		Access: adminroute.Permission("ai_official_model_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Response: officialmodel.OfficialModelDTO{}},
	}, handler.Detail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut, Path: "/api/admin/v1/ai-official-models/:model_id/price",
		Access:   adminroute.Permission("ai_official_model_price_sync"),
		Audit:    adminroute.AuditDecision{Enabled: true, Module: "ai_official_model", Action: "sync_price", Title: "同步官方模型价格"},
		Contract: &adminroute.HTTPContract{Request: updateRequest{}, Response: officialmodel.MutationResponse{}},
	}, handler.SyncPrice)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete, Path: "/api/admin/v1/ai-official-models/:model_id/price-override",
		Access:   adminroute.Permission("ai_official_model_price_sync"),
		Audit:    adminroute.AuditDecision{Enabled: true, Module: "ai_official_model", Action: "restore_official_price", Title: "恢复官方模型价格"},
		Contract: &adminroute.HTTPContract{Query: restoreRequest{}, Response: officialmodel.MutationResponse{}},
	}, handler.RestoreOfficialPrice)
}
