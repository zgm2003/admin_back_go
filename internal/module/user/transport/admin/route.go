package admin

import (
	"net/http"

	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service usermodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/users/me",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Me)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/users/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/users/:id/profile",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: usermodule.ProfileResponse{},
		},
	}, handler.UserProfile)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/users",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/users/export",
		Access: adminroute.Permission("user_userManager_export"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user",
			Action:  "export",
			Title:   "用户导出",
		},
	}, handler.Export)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/users/:id",
		Access: adminroute.Permission("user_userManager_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user",
			Action:  "update",
			Title:   "编辑用户",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/users/:id/status",
		Access: adminroute.Permission("user_userManager_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user",
			Action:  "change_status",
			Title:   "修改用户状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/users",
		Access: adminroute.Permission("user_userManager_batchEdit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user",
			Action:  "batch_update_profile",
			Title:   "批量修改用户资料",
		},
	}, handler.BatchUpdateProfile)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/users/:id",
		Access: adminroute.Permission("user_userManager_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user",
			Action:  "delete",
			Title:   "删除用户",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/users",
		Access: adminroute.Permission("user_userManager_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "user",
			Action:  "delete_batch",
			Title:   "批量删除用户",
		},
	}, handler.DeleteBatch)
}
