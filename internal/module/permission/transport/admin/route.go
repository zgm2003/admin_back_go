package admin

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service ManagementService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewManagementHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/permissions/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/permissions",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/permissions",
		Access: adminroute.Permission("permission_permission_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "permission",
			Action:  "create",
			Title:   "新增权限",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/permissions/:id",
		Access: adminroute.Permission("permission_permission_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "permission",
			Action:  "update",
			Title:   "编辑权限",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/permissions/:id/status",
		Access: adminroute.Permission("permission_permission_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "permission",
			Action:  "change_status",
			Title:   "修改权限状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/permissions/:id",
		Access: adminroute.Permission("permission_permission_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "permission",
			Action:  "delete",
			Title:   "删除权限",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/permissions",
		Access: adminroute.Permission("permission_permission_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "permission",
			Action:  "delete_batch",
			Title:   "批量删除权限",
		},
	}, handler.DeleteBatch)
}
