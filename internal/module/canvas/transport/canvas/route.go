package canvas

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1")
	group.GET("/settings", handler.Settings)
	group.GET("/prompts", handler.Prompts)
	group.GET("/assets", handler.Assets)
}
