package admin

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"

	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

const UIPath = "/api/admin/v1/queue-monitor-ui"

func RegisterRoutes(router *gin.Engine, service HTTPService, monitor http.Handler, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service, monitor)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/queue-monitor",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/queue-monitor/failed",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.FailedList)
	routes.Handle(adminroute.Definition{
		Method: http.MethodConnect,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodHead,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodOptions,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodTrace,
		Path:   "/api/admin/v1/queue-monitor-ui",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodConnect,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodHead,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodOptions,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only queue monitor proxy"),
	}, handler.UI)
	routes.Handle(adminroute.Definition{
		Method: http.MethodTrace,
		Path:   "/api/admin/v1/queue-monitor-ui/*path",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.UI)
}
