package realtime

import "github.com/gin-gonic/gin"

// RegisterRoutes registers admin realtime WebSocket routes.
func RegisterRoutes(router *gin.Engine, handler *Handler) {
	if handler == nil {
		handler = NewHandler(NewService(0), nil, nil, nil)
	}

	v1 := router.Group("/api/admin/v1/realtime")
	v1.GET("/ws", handler.WebSocket)
}
