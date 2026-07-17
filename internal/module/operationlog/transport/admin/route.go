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
		Path:   "/api/admin/v1/operation-logs/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/operation-logs",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/operation-logs/:id",
		Access: adminroute.Permission("devTools_operationLog_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "operation_log",
			Action:  "delete",
			Title:   "删除操作日志",
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/operation-logs",
		Access: adminroute.Permission("devTools_operationLog_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "operation_log",
			Action:  "delete_batch",
			Title:   "批量删除操作日志",
		},
	}, handler.DeleteBatch)
}
