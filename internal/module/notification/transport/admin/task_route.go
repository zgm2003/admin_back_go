package admin

import (
	"net/http"

	notificationtaskmodule "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterTaskRoutes(router *gin.Engine, service notificationtaskmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewTaskHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/notification-tasks/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/notification-tasks/status-count",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.StatusCount)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/notification-tasks",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/notification-tasks",
		Access: adminroute.Permission("system_notificationTask_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "notification_task",
			Action:  "create",
			Title:   "发布通知任务",
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/notification-tasks/:id/cancel",
		Access: adminroute.Permission("system_notificationTask_cancel"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "notification_task",
			Action:  "cancel",
			Title:   "取消通知任务",
		},
	}, handler.Cancel)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/notification-tasks/:id",
		Access: adminroute.Permission("system_notificationTask_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "notification_task",
			Action:  "delete",
			Title:   "删除通知任务",
		},
	}, handler.Delete)
}
