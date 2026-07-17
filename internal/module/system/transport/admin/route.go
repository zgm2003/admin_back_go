package admin

import (
	"net/http"

	systemmodule "admin_back_go/internal/module/system"
	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, readiness systemmodule.ReadinessChecker, routeRegistries ...*adminroute.Registry) {
	service := systemmodule.NewService(readiness)
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/health",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Health)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/ready",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Ready)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ping",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Ping)
}
