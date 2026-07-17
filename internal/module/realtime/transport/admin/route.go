package admin

import (
	"net/http"

	realtimemodule "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

const (
	// WSPath is the admin WebSocket upgrade endpoint.
	WSPath = "/api/admin/v1/realtime/ws"
)

// RegisterRoutes registers admin realtime WebSocket routes.
func RegisterRoutes(router *gin.Engine, handler *Handler, routeRegistries ...*adminroute.Registry) {
	if handler == nil {
		handler = NewHandler(realtimemodule.NewService(0), nil, nil, nil)
	}

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/realtime/ws",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.WebSocket)
}
