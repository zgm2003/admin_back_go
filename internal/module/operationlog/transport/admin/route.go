package admin

import (
	"net/http"

	operationlogmodule "admin_back_go/internal/module/operationlog"
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
		Contract: &adminroute.HTTPContract{
			Response: operationlogmodule.InitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/operation-logs",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    listRequest{},
			Response: operationlogmodule.ListResponse{},
		},
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
		Contract: &adminroute.HTTPContract{
			Response: adminroute.EmptyData{},
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
		Contract: &adminroute.HTTPContract{
			Request:  deleteBatchRequest{},
			Response: adminroute.EmptyData{},
		},
	}, handler.DeleteBatch)
}
