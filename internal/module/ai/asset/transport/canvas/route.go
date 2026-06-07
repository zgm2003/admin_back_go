package canvas

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1")
	group.GET("/assets", handler.Assets)
	group.POST("/assets", handler.Create)
	group.PUT("/assets/:id", handler.Update)
	group.DELETE("/assets/:id", handler.Delete)
}
