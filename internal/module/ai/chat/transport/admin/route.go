package admin

import "github.com/gin-gonic/gin"

func Register(router *gin.Engine, service HTTPService) {
	// AI conversation MVP is WebSocket-only for assistant replies.
	// No /api/admin/v1/ai-chat/* HTTP routes are active in this slice.
}
