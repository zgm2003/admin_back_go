package admin

import (
	"net/http"

	rolemodule "admin_back_go/internal/module/role"
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
		Path:   "/api/admin/v1/roles/page-init",
		Access: adminroute.Permission("permission_role"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: rolemodule.InitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/roles",
		Access: adminroute.Permission("permission_role"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listRequest{},
			Response: rolemodule.ListResponse{},
		},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/roles",
		Access: adminroute.Permission("permission_role_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "role",
			Action:  "create",
			Title:   "新增角色",
		},
		Contract: &adminroute.HTTPContract{
			Request:  mutationRequest{},
			Response: adminroute.IDData{},
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/roles/:id",
		Access: adminroute.Permission("permission_role_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "role",
			Action:  "update",
			Title:   "编辑角色",
		},
		Contract: &adminroute.HTTPContract{
			Request:  mutationRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/roles/:id/default",
		Access: adminroute.Permission("permission_role_setDefault"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "role",
			Action:  "set_default",
			Title:   "设置默认角色",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.SetDefault)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/roles/:id",
		Access: adminroute.Permission("permission_role_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "role",
			Action:  "delete",
			Title:   "删除角色",
		},
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/roles",
		Access: adminroute.Permission("permission_role_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "role",
			Action:  "delete_batch",
			Title:   "批量删除角色",
		},
		Contract: &adminroute.HTTPContract{
			Request:  deleteBatchRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteBatch)
}
