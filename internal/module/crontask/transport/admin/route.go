package admin

import (
	"net/http"

	crontaskmodule "admin_back_go/internal/module/crontask"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service crontaskmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/cron-tasks/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/cron-tasks",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/cron-tasks",
		Access: adminroute.Permission("devTools_cronTask_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "cron_task",
			Action:  "create",
			Title:   "新增定时任务",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/cron-tasks/:id",
		Access: adminroute.Permission("devTools_cronTask_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "cron_task",
			Action:  "update",
			Title:   "编辑定时任务",
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/cron-tasks/:id/status",
		Access: adminroute.Permission("devTools_cronTask_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "cron_task",
			Action:  "change_status",
			Title:   "修改定时任务状态",
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/cron-tasks/:id",
		Access: adminroute.Permission("devTools_cronTask_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "cron_task",
			Action:  "delete",
			Title:   "删除定时任务",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/cron-tasks",
		Access: adminroute.Permission("devTools_cronTask_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "cron_task",
			Action:  "delete_batch",
			Title:   "批量删除定时任务",
		},
	}, handler.DeleteBatch)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/cron-tasks/:id/logs",
		Access: adminroute.Permission("devTools_cronTask_logs"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Logs)
}
