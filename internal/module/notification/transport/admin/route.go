package admin

import (
	"net/http"

	notificationmodule "admin_back_go/internal/module/notification"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service notificationmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/notifications/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/notifications/unread-count",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UnreadCount)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/notifications",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/notifications/:id/read",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service notification state"),
	}, handler.MarkOneRead)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/notifications/read",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service notification state"),
	}, handler.MarkRead)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/notifications/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service notification state"),
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/notifications",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service notification state"),
	}, handler.DeleteBatch)
}
