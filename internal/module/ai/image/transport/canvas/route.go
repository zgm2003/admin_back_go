package canvas

import (
	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aiimagemodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/canvas/v1/ai/images")
	group.POST("/generations", handler.Generations)
	group.POST("/edits", handler.Edits)
	group.GET("/:id", handler.Status)
}
