package admin

import (
	"net/http"

	clientversionmodule "admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service clientversionmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/client-versions/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: clientversionmodule.InitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/client-versions/update-json",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query: updateJSONRequest{},
			ResponseAlternatives: []any{
				clientversionmodule.ManifestPayload{},
				adminroute.EmptyListData{},
			},
		},
	}, handler.UpdateJSON)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/client-versions/current-check",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    currentCheckRequest{},
			Response: clientversionmodule.CurrentCheckResponse{},
		},
	}, handler.CurrentCheck)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/client-versions",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listRequest{},
			Response: clientversionmodule.ListResponse{},
		},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/client-versions",
		Access: adminroute.Permission("system_clientVersion_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "client_version",
			Action:  "create",
			Title:   "发布客户端版本",
		},
		Contract: &adminroute.HTTPContract{
			Request:  saveRequest{},
			Response: adminroute.IDData{},
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/client-versions/:id",
		Access: adminroute.Permission("system_clientVersion_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "client_version",
			Action:  "update",
			Title:   "编辑客户端版本",
		},
		Contract: &adminroute.HTTPContract{
			Request:  saveRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/client-versions/:id/latest",
		Access: adminroute.Permission("system_clientVersion_setLatest"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "client_version",
			Action:  "set_latest",
			Title:   "设为最新版本",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.SetLatest)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/client-versions/:id/force-update",
		Access: adminroute.Permission("system_clientVersion_forceUpdate"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "client_version",
			Action:  "force_update",
			Title:   "切换强制更新",
		},
		Contract: &adminroute.HTTPContract{
			Request:  forceUpdateRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.ForceUpdate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/client-versions/:id",
		Access: adminroute.Permission("system_clientVersion_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "client_version",
			Action:  "delete",
			Title:   "删除客户端版本",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.Delete)
}
