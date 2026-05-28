package admin

import (
	realtimemodule "admin_back_go/internal/module/realtime"

	"github.com/gin-gonic/gin"
)

const (
	// WSPath is the admin WebSocket upgrade endpoint.
	WSPath = "/api/admin/v1/realtime/ws"
)

// RegisterRoutes registers admin realtime WebSocket routes.
func RegisterRoutes(router *gin.Engine, handler *Handler) {
	if handler == nil {
		handler = NewHandler(realtimemodule.NewService(0), nil, nil, nil)
	}

	router.GET(WSPath, handler.WebSocket)
}
