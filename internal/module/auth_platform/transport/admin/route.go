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
		Path:   "/api/admin/v1/auth-platforms/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/auth-platforms",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/auth-platforms",
		Access: adminroute.Permission("permission_authPlatform_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "auth_platform",
			Action:  "create",
			Title:   "新增认证平台",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/auth-platforms/:id",
		Access: adminroute.Permission("permission_authPlatform_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "auth_platform",
			Action:  "update",
			Title:   "编辑认证平台",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/auth-platforms/:id/status",
		Access: adminroute.Permission("permission_authPlatform_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "auth_platform",
			Action:  "change_status",
			Title:   "修改认证平台状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/auth-platforms/:id",
		Access: adminroute.Permission("permission_authPlatform_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "auth_platform",
			Action:  "delete",
			Title:   "删除认证平台",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/auth-platforms",
		Access: adminroute.Permission("permission_authPlatform_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "auth_platform",
			Action:  "delete_batch",
			Title:   "批量删除认证平台",
		},
	}, handler.DeleteBatch)
}
