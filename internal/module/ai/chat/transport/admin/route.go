package admin

import (
	aichatmodule "admin_back_go/internal/module/ai/chat"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aichatmodule.HTTPService) {
	// AI conversation MVP is WebSocket-only for assistant replies.
	// No /api/admin/v1/ai-chat/* HTTP routes are active in this slice.
}
