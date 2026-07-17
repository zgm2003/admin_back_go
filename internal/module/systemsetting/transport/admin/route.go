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
		Path:   "/api/admin/v1/system-settings/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/system-settings",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/system-settings",
		Access: adminroute.Permission("system_setting_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "create",
			Title:   "新增系统设置",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/system-settings/:id",
		Access: adminroute.Permission("system_setting_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "update",
			Title:   "编辑系统设置",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/system-settings/:id/status",
		Access: adminroute.Permission("system_setting_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "change_status",
			Title:   "修改系统设置状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/system-settings/:id",
		Access: adminroute.Permission("system_setting_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "delete",
			Title:   "删除系统设置",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/system-settings",
		Access: adminroute.Permission("system_setting_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "delete_batch",
			Title:   "批量删除系统设置",
		},
	}, handler.DeleteBatch)
}
